/* eslint-disable react-hooks/set-state-in-effect */
// Reinicia el borrador local cuando se abre para otro producto (patrón legítimo).
import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { DatePickerButton } from '../components/DatePickerButton';
import { Box, VStack, HStack, Text, Button, Input } from '@chakra-ui/react';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter,
  DialogTitle, DialogCloseTrigger,
} from '../components/ui/dialog';
import { Field } from '../components/ui/field';
import { Switch } from '../components/ui/switch';
import { toaster } from '../components/ui/toaster';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { medirAccion } from '../api/uso';
import { adminApi, type AdminProduct, type Category, type CostChange } from '../api/admin';
import { montoTecleado } from '../domain/numeros';
import { moneyExact } from '../utils/format';
import { Picker, type PickerOption } from '../components/Picker';
import { useUiStore } from '../stores/ui';
import { ProductGroupsManager } from './ProductGroupsManager';
import { CompositionSheet } from './CompositionSheet';
import { SeccionAlertas } from './alertas/SeccionAlertas';
import { can } from '../app/permissions';
import { compositionLabel } from './compositionLabel';
import { AltNamesField } from '../components/AltNamesField';
import { altNameKey, withPendingAlt } from '../components/altNames';
import { ApiError } from '../api/client';
import { DosColumnas, LugaresDeVenta, MenuDePestanas, PanelVentaEnWeb, VenderSinExistencia, type Pestana } from './producto/PanelesDeVenta';
import { precioWebValido, ventaInicial, ventaParaGuardar } from './producto/ventaDelProducto';

// Las pestañas de la ficha (spec 092, maqueta G2). Nombres de uso común, elegidos por el dueño.
const PESTANAS: Pestana[] = [
  { id: 'general', label: 'General' },
  { id: 'disponibilidad', label: 'Disponibilidad' },
  { id: 'web', label: 'Venta en web' },
  { id: 'receta', label: 'Receta e inventario' },
  { id: 'extras', label: 'Extras' },
];

interface Props {
  product: AdminProduct | null;
  isOpen: boolean;
  onClose: () => void;
}

// Diálogo de edición de producto reutilizable (Productos admin + modo editar del POS).
// Guarda vía adminApi e invalida menú + lista admin → el POS refleja el cambio al instante.
export function ProductEditDialog({ product, isOpen, onClose }: Props) {
  const qc = useQueryClient();
  const palette = useUiStore((s) => s.palette);
  const [edit, setEdit] = useState<AdminProduct | null>(product);
  const [costMode, setCostMode] = useState<'manual' | 'receta'>(modoInicial(product));
  const [costText, setCostText] = useState(product?.manualCost ?? '');
  // Otros nombres (spec 063): se guardan con «Guardar», por su propia ruta y antes del PATCH.
  const [altNames, setAltNames] = useState<string[]>(product?.altNames ?? []);
  const [altDraft, setAltDraft] = useState('');
  const [altError, setAltError] = useState('');
  // Dónde y cómo se vende (spec 092): un solo estado para la casilla Web de Disponibilidad y el
  // interruptor de Venta en web, así nunca dicen cosas distintas.
  const [venta, setVenta] = useState(ventaInicial(product));
  const [pestana, setPestana] = useState('general');
  const [errores, setErrores] = useState<Set<string>>(new Set());
  const [intento, setIntento] = useState(false);
  useEffect(() => {
    setEdit(product);
    setCostMode(modoInicial(product));
    setCostText(product?.manualCost ?? '');
    setAltNames(product?.altNames ?? []);
    setAltDraft('');
    setAltError('');
    setVenta(ventaInicial(product));
    setPestana('general');
    setErrores(new Set());
    setIntento(false);
  }, [product]);
  const similar = useCallback((name: string) => adminApi.similarProductAltNames(product?.id ?? 0, name), [product?.id]);
  const cost = edit ? cambioDeCosto(edit, costMode, costText) : undefined;
  const [composing, setComposing] = useState(false);
  // Solo se piden al abrir el diálogo: es un catálogo chico que casi nunca cambia y no vale un
  // viaje por cada producto que se lista.
  const { data: cats } = useQuery({
    queryKey: ['admin', 'categories'],
    queryFn: adminApi.categories,
    enabled: isOpen,
    staleTime: 5 * 60 * 1000,
  });
  const categorias = cats?.items ?? [];

  const save = useMutation({
    mutationFn: async (p: AdminProduct) => {
      const before = product?.altNames ?? [];
      const oldName = product?.name ?? p.name;
      // Lo escrito sin tocar «Agregar» se agrega aquí: en 600 px «Guardar» queda lejos de «Agregar»
      // y perderlo en silencio era lo más probable (revisión de tableta, A1). El nombre nuevo nunca
      // va como otro nombre: subir un otro nombre a nombre principal lo saca de la lista.
      const wanted = withPendingAlt(altNames, altDraft, p.name).filter((a) => altNameKey(a) !== altNameKey(p.name));
      // El servidor compara contra el nombre GUARDADO: la lista que conserva el nombre viejo (corregir
      // un dedazo y dejarlo como otro nombre) va después del PATCH, y el resto antes (revisión).
      const first = wanted.filter((a) => altNameKey(a) !== altNameKey(oldName));
      const put = async (list: string[]) => {
        try {
          const saved = await adminApi.setProductAltNames(p.id, list);
          setAltNames(saved.altNames);
          setAltDraft('');
          // Guardada la lista, la de Productos ya cambió aunque falle lo que sigue.
          void qc.invalidateQueries({ queryKey: ['admin', 'products'] });
        } catch (e) {
          throw new AltSaveError(e);
        }
      };
      if (!sameList(first, before)) await put(first);
      await adminApi.updateProduct(p.id, {
        name: p.name, price: Number(p.price), favorite: p.is_favorite, active: p.is_active,
        needsPrep: p.needsPrep, categoryId: p.categoryId,
        availableFrom: p.availableFrom, availableUntil: p.availableUntil,
        ...(cost && cost !== 'invalido' ? { cost } : {}),
        sales: ventaParaGuardar(venta, product?.webPrice ?? null),
      });
      if (!sameList(wanted, first)) await put(wanted);
    },
    onSuccess: () => {
      // Después de que el servidor guardó, nunca antes (spec 017, US3).
      medirAccion('catalogo', 'editar-producto');
      qc.invalidateQueries({ queryKey: ['admin', 'products'] });
      qc.invalidateQueries({ queryKey: ['menu'] });
      onClose();
      toaster.create({ title: 'Producto actualizado', type: 'success' });
    },
    onError: (e) => {
      // Lo que rechazó la lista se ve bajo su campo y la ficha no se cierra; un choque del nombre
      // (PATCH) no es de la lista y va como cualquier otro error.
      if (e instanceof AltSaveError) {
        const inner = e.cause;
        setAltError(inner instanceof ApiError && inner.status < 500 ? inner.message : 'No se pudieron guardar los otros nombres');
        return;
      }
      toaster.create({ title: 'Error', description: e instanceof ApiError && e.status < 500 ? e.message : String(e), type: 'error' });
    },
  });

  // Guardar siempre se puede tocar: si algo no es válido, la ficha va a la pestaña del error y la
  // marca, en vez de un botón apagado que no dice por qué (revisión de tableta, 092).
  const erroresDe = (): Set<string> => {
    const conError = new Set<string>();
    // Un precio de mostrador vacío o en cero no es un precio: guardarlo dejaría el producto gratis.
    const precio = edit ? montoTecleado(String(edit.price)) : undefined;
    if (cost === 'invalido' || precio === undefined || precio <= 0) conError.add('general');
    if (venta.web && !precioWebValido(venta)) conError.add('web');
    return conError;
  };
  // Tras el primer intento, las marcas se recalculan con cada cambio: corregir el campo quita el
  // punto rojo sin tocar Guardar otra vez.
  const marcas = intento ? erroresDe() : errores;
  const guardar = () => {
    if (!edit) return;
    const conError = erroresDe();
    setIntento(true);
    setErrores(conError);
    const primera = PESTANAS.find((t) => conError.has(t.id));
    if (primera) {
      setPestana(primera.id);
      return;
    }
    save.mutate(edit);
  };

  return (
    <DialogRoot open={isOpen} onOpenChange={(e) => { if (!e.open) onClose(); }} size="full" scrollBehavior="inside">
      <DialogBackdrop />
      <DialogContent colorPalette={palette}>
        <DialogHeader><DialogTitle>Editar producto</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        {edit && (
          <>
            <DialogBody display="flex" gap={6} pt={0}>
              <MenuDePestanas pestanas={PESTANAS} activa={pestana} onElegir={setPestana} conError={marcas} />
              <Box flex="1" minW={0} py={3} role="tabpanel" aria-label={PESTANAS.find((t) => t.id === pestana)?.label}>
                {pestana === 'general' && (
                  <DosColumnas>
                    <Box><Field label="Nombre">
                  <Input minH="44px" value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} />
                </Field></Box>
                    <Box><HStack align="start" gap={3}>
                <Field flex="1" label="Precio">
                  <Input type="number" inputMode="decimal" minH="44px" value={edit.price}
                    onChange={(e) => setEdit({ ...edit, price: e.target.value })} />
                </Field>
                <Box flex="1">
                {edit.type === 'combo' ? (
                  <CostoFijo monto={edit.current_cost} nota="Se suma de lo que incluye" />
                ) : costMode === 'receta' ? (
                  // Recién elegido, current_cost todavía es el manual: mostrarlo con esta nota sería
                  // un número falso. El servidor lo calcula al guardar.
                  <CostoFijo monto={edit.costSource === 'receta' ? edit.current_cost : undefined}
                    nota={edit.costSource === 'receta' ? 'Sale de su receta' : 'Se calculará de su receta al guardar'}>
                    <Button variant="outline" size="sm" minH="44px" onClick={() => setCostMode('manual')}>Poner costo a mano</Button>
                  </CostoFijo>
                ) : (
                  <VStack align="stretch" gap={2}>
                    <Field label="Costo" invalid={cost === 'invalido'} errorText="Escribe el costo">
                      <Input type="number" inputMode="decimal" minH="44px" value={costText}
                        onChange={(e) => setCostText(e.target.value)} />
                    </Field>
                    {edit.hasRecipe && (
                      <Button variant="outline" size="sm" minH="44px" onClick={() => setCostMode('receta')}>Usar el de su receta</Button>
                    )}
                  </VStack>
                )}
                </Box>
                </HStack></Box>
                    <Box><Field label="Categoría">
                  {/* Picker táctil, no <select> nativo: en una tablet de 7" el desplegable del
                      sistema tapa la pantalla con renglones de 20px. Ver la constitución. */}
                  <Picker
                    value={String(edit.categoryId || '')}
                    onChange={(v) => setEdit({ ...edit, categoryId: Number(v) })}
                    options={opcionesDeCategoria(categorias, edit)}
                    placeholder="Sin categoría"
                    title="Mover a la categoría"
                  />
                </Field></Box>
                    <Box><AltNamesField ownName={edit.name} value={altNames} onChange={(v) => { setAltNames(v); setAltError(''); }}
                  draft={altDraft} onDraftChange={(d) => { setAltDraft(d); setAltError(''); }} similar={similar} error={altError} /></Box>
                    <VStack align="stretch" gap={3}><HStack justify="space-between">
                  <Text>Favorito</Text>
                  <Switch size="lg" py="10px" checked={edit.is_favorite} onCheckedChange={(e) => setEdit({ ...edit, is_favorite: e.checked })} />
                </HStack>
                <HStack justify="space-between">
                  <Text>Activo</Text>
                  <Switch size="lg" py="10px" checked={edit.is_active} onCheckedChange={(e) => setEdit({ ...edit, is_active: e.checked })} />
                </HStack></VStack>
                    <Box>{/* Lo que decide si el producto manda su pedido al tablero de Pedidos. Nace
                    encendido, así que apagarlo es la excepción y no hay que revisar 1004 fichas. */}
                <HStack justify="space-between" align="start">
                  <Box pr={3}>
                    <Text>Pasa por cocina</Text>
                    <Text fontSize="xs" color="fg.muted">
                      Apágalo para lo que se toma tal cual, como una bebida de la nevera. Un pedido
                      que solo lleve cosas apagadas se cobra y ya, sin pasar por Pedidos.
                    </Text>
                  </Box>
                  <Switch size="lg" py="10px" checked={edit.needsPrep}
                    onCheckedChange={(e) => setEdit({ ...edit, needsPrep: e.checked })} />
                </HStack></Box>
                  </DosColumnas>
                )}
                {pestana === 'disponibilidad' && (
                  <VStack align="stretch" gap={6} maxW="640px">
                    <Box>
                      <Text fontSize="sm" fontWeight="600" mb={2}>Dónde se vende</Text>
                      <LugaresDeVenta venta={venta} setVenta={setVenta} />
                    </Box>
                    <Box>
                  <Text fontSize="sm" fontWeight="600" mb={1}>Temporada — opcional</Text>
                  <Text fontSize="xs" color="fg.muted" mb={2}>Con fechas, el producto solo aparece en el POS dentro del rango. Vacío = siempre.</Text>
                  <HStack>
                    <Field label="Desde">
                      <DatePickerButton label="Desde" clearable value={edit.availableFrom ?? ''}
                        onChange={(v) => setEdit({ ...edit, availableFrom: v || null })} />
                    </Field>
                    <Field label="Hasta">
                      <DatePickerButton label="Hasta" clearable value={edit.availableUntil ?? ''}
                        onChange={(v) => setEdit({ ...edit, availableUntil: v || null })} />
                    </Field>
                  </HStack>
                </Box>
                  </VStack>
                )}
                {pestana === 'web' && (
                  <PanelVentaEnWeb venta={venta} setVenta={setVenta} precioMostrador={edit.price} errorPrecio={marcas.has('web')} />
                )}
                {pestana === 'receta' && (
                  <VStack align="stretch" gap={5} maxW="640px">
                    <Button variant="outline" minH="44px" justifyContent="space-between" onClick={() => setComposing(true)}>
                  <Text>Receta</Text>
                  <Text color="fg.muted" fontWeight="normal">{compositionLabel(edit.compositionStatus)} ›</Text>
                </Button>
                    <VenderSinExistencia venta={venta} setVenta={setVenta} mide={edit.measuresStock ?? false} />
                    {/* Alertas de stock bajo (spec 084): solo aparece si el producto lleva existencia propia
                    (el servidor responde 404 a los demás) y quien edita puede ver el almacén. */}
                {can('inventory.view') && (
                  <SeccionAlertas itemType="producto" id={edit.id} name={edit.name} kind="pieza" canConfigure={can('inventory.configure')} />
                )}
                  </VStack>
                )}
                {pestana === 'extras' && (
                  <Box maxW="760px">
                    <Text fontSize="sm" fontWeight="600" mb={3}>Grupos modificadores</Text>
                    <ProductGroupsManager productId={edit.id} />
                  </Box>
                )}
              </Box>
            </DialogBody>
            <DialogFooter>
              <Button variant="ghost" minH="44px" mr={3} onClick={onClose}>Cancelar</Button>
              <Button minH="44px" loading={save.isPending} onClick={guardar}>Guardar</Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
      {edit && (
        <CompositionSheet kind="product" id={edit.id} name={edit.name} open={composing}
          onClose={() => setComposing(false)} />
      )}
    </DialogRoot>
  );
}

// Las subcategorías se leen con su padre ("Bebidas › Calientes") para desambiguar dos "Calientes".
// Mientras el catálogo no llega se pinta la categoría ACTUAL del producto: un picker vacío sobre el
// producto que estás editando parece que lo borró.
function opcionesDeCategoria(cats: Category[], producto: AdminProduct): PickerOption[] {
  if (cats.length === 0) return [{ value: String(producto.categoryId), label: producto.category }];
  const nombrePorId = new Map(cats.map((c) => [c.id, c.name]));
  return cats.map((c) => ({
    value: String(c.id),
    label: c.parentId ? `${nombrePorId.get(c.parentId) ?? '—'} › ${c.name}` : c.name,
  }));
}

// Un producto con receta arranca mostrando el costo calculado; «compra» y «manual» se editan igual.
function modoInicial(p: AdminProduct | null): 'manual' | 'receta' {
  return p?.costSource === 'receta' ? 'receta' : 'manual';
}

// cambioDeCosto decide qué viaja al guardar: nada si el costo no cambió (un guardado de nombre no
// reescribe el costo), y 'invalido' si el campo quedó vacío o malformado — un vacío no es cero.
function cambioDeCosto(p: AdminProduct, modo: 'manual' | 'receta', texto: string): CostChange | 'invalido' | undefined {
  if (p.type === 'combo') return undefined;
  if (modo === 'receta') return p.costSource === 'receta' ? undefined : { source: 'receta' };
  const monto = texto.trim() === '' ? undefined : montoTecleado(texto);
  if (p.costSource !== 'receta' && texto === (p.manualCost ?? '')) return undefined;
  if (monto === undefined || monto < 0) return 'invalido';
  return { source: 'manual', amount: monto };
}

// CostoFijo muestra un costo que no se captura aquí (receta o combo) con la razón al lado.
export function CostoFijo({ monto, nota, children }: { monto?: string; nota: string; children?: ReactNode }) {
  return (
    <VStack align="stretch" gap={1}>
      <Text fontSize="sm" fontWeight="500">Costo</Text>
      <HStack gap={2} minH="44px">
        {monto !== undefined && <Text fontWeight="600">{moneyExact(monto)}</Text>}
        <Text fontSize="xs" color="fg.muted">{nota}</Text>
      </HStack>
      {children}
    </VStack>
  );
}

// AltSaveError marca que lo que falló fue guardar la lista de otros nombres, no el resto de la ficha.
class AltSaveError extends Error {
  constructor(cause: unknown) {
    super('alt-names', { cause });
  }
}

const sameList = (a: string[], b: string[]) => a.length === b.length && a.every((x, i) => x === b[i]);
