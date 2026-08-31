/* eslint-disable react-hooks/set-state-in-effect */
// Reinicia el borrador local cuando se abre para otro producto (patrón legítimo).
import { useEffect, useState } from 'react';
import { Box, VStack, HStack, Text, Button, Input } from '@chakra-ui/react';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter,
  DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { Field } from '../../components/ui/field';
import { Switch } from '../../components/ui/switch';
import { toaster } from '../../components/ui/toaster';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminApi, type AdminProduct, type Category } from '../../api/admin';
import { Picker, type PickerOption } from '../../components/Picker';
import { useUiStore } from '../../stores/ui';
import { ProductGroupsManager } from './ProductGroupsManager';

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
  useEffect(() => { setEdit(product); }, [product]);
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
    mutationFn: (p: AdminProduct) =>
      adminApi.updateProduct(p.id, {
        name: p.name, price: Number(p.price), favorite: p.is_favorite, active: p.is_active,
        needsPrep: p.needsPrep, categoryId: p.categoryId,
        availableFrom: p.availableFrom, availableUntil: p.availableUntil,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin', 'products'] });
      qc.invalidateQueries({ queryKey: ['menu'] });
      onClose();
      toaster.create({ title: 'Producto actualizado', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  return (
    <DialogRoot open={isOpen} onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DialogBackdrop />
      <DialogContent colorPalette={palette}>
        <DialogHeader><DialogTitle>Editar producto</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        {edit && (
          <>
            <DialogBody>
              <VStack align="stretch" gap={4}>
                <Field label="Nombre">
                  <Input value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} />
                </Field>
                <Field label="Precio">
                  <Input type="number" value={edit.price}
                    onChange={(e) => setEdit({ ...edit, price: e.target.value })} />
                </Field>
                <Field label="Categoría">
                  {/* Picker táctil, no <select> nativo: en una tablet de 7" el desplegable del
                      sistema tapa la pantalla con renglones de 20px. Ver la constitución. */}
                  <Picker
                    value={String(edit.categoryId || '')}
                    onChange={(v) => setEdit({ ...edit, categoryId: Number(v) })}
                    options={opcionesDeCategoria(categorias, edit)}
                    placeholder="Sin categoría"
                    title="Mover a la categoría"
                  />
                </Field>
                <HStack justify="space-between">
                  <Text>Favorito</Text>
                  <Switch checked={edit.is_favorite} onCheckedChange={(e) => setEdit({ ...edit, is_favorite: e.checked })} />
                </HStack>
                <HStack justify="space-between">
                  <Text>Activo</Text>
                  <Switch checked={edit.is_active} onCheckedChange={(e) => setEdit({ ...edit, is_active: e.checked })} />
                </HStack>
                {/* Lo que decide si el producto manda su pedido al tablero de Pedidos. Nace
                    encendido, así que apagarlo es la excepción y no hay que revisar 1004 fichas. */}
                <HStack justify="space-between" align="start">
                  <Box pr={3}>
                    <Text>Pasa por cocina</Text>
                    <Text fontSize="xs" color="fg.muted">
                      Apágalo para lo que se toma tal cual, como una bebida de la nevera. Un pedido
                      que solo lleve cosas apagadas se cobra y ya, sin pasar por Pedidos.
                    </Text>
                  </Box>
                  <Switch checked={edit.needsPrep}
                    onCheckedChange={(e) => setEdit({ ...edit, needsPrep: e.checked })} />
                </HStack>
                <Box>
                  <Text fontSize="sm" fontWeight="600" mb={1}>Disponibilidad (temporada) — opcional</Text>
                  <Text fontSize="xs" color="fg.muted" mb={2}>Con fechas, el producto solo aparece en el POS dentro del rango. Vacío = siempre.</Text>
                  <HStack>
                    <Field label="Desde">
                      <Input type="date" value={edit.availableFrom ?? ''}
                        onChange={(e) => setEdit({ ...edit, availableFrom: e.target.value || null })} />
                    </Field>
                    <Field label="Hasta">
                      <Input type="date" value={edit.availableUntil ?? ''}
                        onChange={(e) => setEdit({ ...edit, availableUntil: e.target.value || null })} />
                    </Field>
                  </HStack>
                </Box>
                <Box borderTopWidth="1px" pt={4}>
                  <Text fontSize="sm" fontWeight="600" mb={1}>Grupos modificadores</Text>
                  <Text fontSize="xs" color="fg.muted" mb={3}>Los cambios de grupos se aplican al instante (no dependen de «Guardar»).</Text>
                  <ProductGroupsManager productId={edit.id} />
                </Box>
              </VStack>
            </DialogBody>
            <DialogFooter>
              <Button variant="ghost" mr={3} onClick={onClose}>Cancelar</Button>
              <Button loading={save.isPending} onClick={() => save.mutate(edit)}>Guardar</Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
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
