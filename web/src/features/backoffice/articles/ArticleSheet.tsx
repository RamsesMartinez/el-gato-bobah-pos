import { useEffect, useState } from 'react';
import { medirAccion } from '../../../api/uso';
import { Badge, Box, Button, HStack, Input, SimpleGrid, Spinner, Stack, Text } from '@chakra-ui/react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader,
} from '../../../components/ui/drawer';
import { backofficeApi, type ArticleSheet as Sheet, type ArticleUpdate } from '../../../api/backoffice';
import { ApiError } from '../../../api/client';
import { Picker } from '../../../components/Picker';
import { ConfirmSheet } from '../../../components/ConfirmSheet';
import { useAutosave, type AutosaveStatus } from './useAutosave';
import { useLocations } from '../useLocations';
import { toaster } from '../../../components/ui/toaster';
import { can } from '../../../app/permissions';
import { useSessionStore } from '../../../stores/session';
import { useHoraDelNegocio } from '../../../hooks/useHoraDelNegocio';
import { formatBaseQty, formatUnitCost, packExample } from '../../../shared/units';
import { CompositionSheet } from '../../../shared/CompositionSheet';
import { PresentationsSheet } from '../PresentationsSheet';
import { CostTrend } from './CostTrend';
import { ArticleAuditSheet } from './ArticleAuditSheet';
import { ArchiveSheet } from './ArchiveSheet';
import { UnitChangeSheet } from './UnitChangeSheet';
import { QtyWithUnit } from './qty';
import { baseQty, bigUnitOf, KIND_WORD, type Kind } from './articleUnits';

// La ficha de un artículo (spec 042, maqueta «Ficha del artículo», opción A: dos columnas). A la
// izquierda lo que se edita y los empaques; a la derecha el costo, su tendencia y el historial. De pie
// (menos de 800 px de ancho) es una sola columna con el costo arriba (revisión de tableta, M2).
// Guarda sola (spec 043, maqueta «Ficha del artículo»): cada campo al elegir, tras 2 s sin escribir o
// al salir de él; un indicador junto al título dice en qué va.

interface Props {
  id: number;
  // Recién creado: «Agregar empaque» va primero, a la vista (decisión del dueño, FR-002).
  justCreated?: boolean;
  onClose: () => void;
}

interface Form {
  name: string;
  categoryId: string;
  placeId: string;
  min: { qty: string; big: boolean };
}

// Saved son los valores tal como se guardan: el mínimo en la unidad base, '' para vacío.
type Saved = { name: string; categoryId: string; placeId: string; min: string };

function savedOf(a: Sheet): Saved {
  return {
    name: a.name,
    categoryId: a.categoryId == null ? '' : String(a.categoryId),
    placeId: a.countPlaceId == null ? '' : String(a.countPlaceId),
    min: a.minStock == null ? '' : String(Number(a.minStock)),
  };
}

const NUMBER = /^\d+(\.\d+)?$/;

function minDisplay(base: string, kind: Kind): { qty: string; big: boolean } {
  const big = bigUnitOf(kind);
  if (base === '') return { qty: '', big: !!big };
  return big && Number(base) >= big.factor ? { qty: String(Number(base) / big.factor), big: true } : { qty: base, big: false };
}

function formOf(a: Sheet): Form {
  const big = bigUnitOf(a.baseUnitKind as Kind);
  const min = a.minStock == null ? '' : big && Number(a.minStock) >= big.factor
    ? String(Number(a.minStock) / big.factor) : String(Number(a.minStock));
  return {
    name: a.name,
    categoryId: a.categoryId == null ? '' : String(a.categoryId),
    placeId: a.countPlaceId == null ? '' : String(a.countPlaceId),
    min: { qty: min, big: !!big && a.minStock != null && Number(a.minStock) >= big.factor },
  };
}

export function ArticleSheet({ id, justCreated, onClose }: Props) {
  const qc = useQueryClient();
  const user = useSessionStore((s) => s.user);
  const canConfigure = can('inventory.configure', user);
  const canReceive = can('inventory.receive', user);
  const canView = can('inventory.view', user);
  const hora = useHoraDelNegocio();
  const art = useQuery({ queryKey: ['article', id], queryFn: () => backofficeApi.article(id) });
  const cats = useQuery({ queryKey: ['article-categories'], queryFn: backofficeApi.articleCategories });
  const locations = useLocations();
  const units = useQuery({ queryKey: ['units'], queryFn: backofficeApi.units, enabled: canConfigure });
  const allPres = useQuery({ queryKey: ['presentations'], queryFn: backofficeApi.presentations, enabled: canReceive });
  const history = useInfiniteQuery({
    queryKey: ['article-cost-history', id],
    queryFn: ({ pageParam }) => backofficeApi.articleCostHistory(id, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (p) => p.next ?? undefined,
  });
  const [form, setForm] = useState<Form | null>(null);
  const [panel, setPanel] = useState<'audit' | 'archive' | 'packs' | 'recipe' | 'cost' | 'discard' | 'unit' | null>(null)
  const [editPack, setEditPack] = useState<number | undefined>(undefined);
  const [cost, setCost] = useState('');

  // El formulario se llena solo con la ficha pedida al abrir, nunca con la de la caché: la caché puede
  // traer el nombre de antes de un guardado y el campo lo mostraría como si fuera lo vigente.
  const fresh = art.isFetchedAfterMount ? art.data : undefined;
  useEffect(() => {
    if (fresh && form === null) setForm(formOf(fresh));
  }, [fresh, form]);

  const a = art.data;
  const kind = (a?.baseUnitKind ?? 'pieza') as Kind;
  const big = bigUnitOf(kind);

  const auto = useAutosave<Saved>({
    initial: a ? savedOf(a) : { name: '', categoryId: '', placeId: '', min: '' },
    version: a?.updatedAt ?? '',
    // Manda la ficha completa: lo que cambió más lo último guardado de lo demás. La API anterior, que
    // atiende durante una subida, toma un campo ausente como vacío y lo borraría. La nueva compara
    // contra lo que había y deja en la bitácora solo lo que cambió.
    save: async (patch, version, saved) => {
      const v = { ...saved, ...patch };
      const body: ArticleUpdate = {
        expectedUpdatedAt: version, name: v.name,
        categoryId: v.categoryId ? Number(v.categoryId) : null,
        placeId: v.placeId ? Number(v.placeId) : null,
        minStock: v.min === '' ? null : v.min,
      };
      const r = await backofficeApi.updateArticle(id, body);
      medirAccion('inventario', 'editar-articulo');
      // La API anterior (durante una subida) responde sin la versión nueva: se pide la ficha.
      const updatedAt = r?.updatedAt ?? (await qc.fetchQuery({ queryKey: ['article', id], queryFn: () => backofficeApi.article(id), staleTime: 0 })).updatedAt;
      void qc.invalidateQueries({ queryKey: ['articles'] });
      void qc.invalidateQueries({ queryKey: ['article', id] });
      void qc.invalidateQueries({ queryKey: ['article-audit', id] });
      return { updatedAt };
    },
    validate: (k, v) => {
      if (k === 'name' && v.trim() === '') return 'El nombre no puede quedar vacío.';
      if (k === 'min' && v !== '' && !(NUMBER.test(v))) return 'Escribe el mínimo con números.';
      return null;
    },
    complete: (k, v) => k !== 'min' || NUMBER.test(v),
    onReject: (k, saved, e) => {
      setForm((f) => f && (k === 'min' ? { ...f, min: minDisplay(saved, kind) } : { ...f, [k === 'categoryId' ? 'categoryId' : k === 'placeId' ? 'placeId' : 'name']: saved }));
      toaster.create({ title: e instanceof ApiError && e.status < 500 ? e.message : 'No se pudo guardar', type: 'error' });
    },
    onStale: () => {
      toaster.create({ title: 'Cambió mientras la veías', description: 'Se volvió a cargar con lo último.', type: 'warning' });
      void refresh();
    },
  });
  const [synced, setSynced] = useState(false);
  useEffect(() => {
    if (fresh && !synced) { auto.reset(savedOf(fresh), fresh.updatedAt); setSynced(true); }
  }, [fresh, synced, auto]);

  // minValue es lo que el mínimo manda: la cantidad base si ya es un número, o lo escrito tal cual para
  // que la cola lo valide (y no lo mande por la pausa).
  const minValue = (m: { qty: string; big: boolean }) => (m.qty === '' ? '' : NUMBER.test(m.qty) ? baseQty(m.qty, kind, m.big) ?? m.qty : m.qty);

  // refresh llena el formulario con la ficha que ya trae lo guardado, después de una hoja que la cambió
  // (costo, unidad, archivar, empaques). Vaciarlo antes dejaba que el efecto de arriba lo volviera a
  // llenar con la caché vieja: el mínimo regresaba a su valor anterior (#120).
  // Un campo que quedó sin guardar (marcado o fallido) conserva lo escrito: volver de una hoja no lo
  // borra sin aviso (R1).
  const refresh = async () => {
    const fresh = await qc.fetchQuery({ queryKey: ['article', id], queryFn: () => backofficeApi.article(id), staleTime: 0 });
    const keep = new Set<string>(auto.unsaved as string[]);
    setForm((prev) => {
      const next = formOf(fresh);
      if (!prev) return next;
      return {
        name: keep.has('name') ? prev.name : next.name,
        categoryId: keep.has('categoryId') ? prev.categoryId : next.categoryId,
        placeId: keep.has('placeId') ? prev.placeId : next.placeId,
        min: keep.has('min') ? prev.min : next.min,
      };
    });
    if (keep.size === 0) auto.reset(savedOf(fresh), fresh.updatedAt);
    else auto.rebase(fresh.updatedAt);
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['articles'] }),
      qc.invalidateQueries({ queryKey: ['article-cost-history', id] }),
      qc.invalidateQueries({ queryKey: ['article-audit', id] }),
    ]);
  };
  const correct = useMutation({
    mutationFn: () => backofficeApi.correctArticleCost(id, String(Number(cost) / (big?.factor ?? 1))),
    onSuccess: async () => { medirAccion('inventario', 'corregir-costo'); setPanel(null); setCost(''); await refresh(); toaster.create({ title: 'Costo corregido', type: 'success' }); },
    onError: (e) => toaster.create({ title: e instanceof ApiError && e.status !== 500 ? e.message : 'No se pudo corregir el costo', type: 'error' }),
  });

  // close es la única salida («Volver a Artículos», el fondo, Escape): guarda el campo que tiene el foco
  // (en la tableta tocar un botón no siempre lo quita), espera la cola y pregunta solo si algo quedó
  // sin guardar (R2).
  const [leaving, setLeaving] = useState(false);
  const close = async () => {
    if (leaving) return;
    const el = document.activeElement;
    if (el instanceof HTMLElement && el.tagName === 'INPUT') el.blur();
    setLeaving(true);
    const allSaved = await auto.whenIdle();
    setLeaving(false);
    if (allSaved) onClose();
    else setPanel('discard');
  };
  const items = history.data?.pages.flatMap((p) => p.items ?? []) ?? [];
  const trend = history.data?.pages[0]?.trendPct;
  const packs = (allPres.data ?? []).filter((p) => p.ingredientId === id);

  return (
    <DrawerRoot open placement="bottom" size="full" onOpenChange={(e) => { if (!e.open) void close(); }}>
      <DrawerBackdrop />
      <DrawerContent display="flex" flexDirection="column">
        <DrawerHeader py={2}>
          <HStack gap={2}>
            <Button minH="44px" variant="outline" flexShrink={0} loading={leaving} onClick={() => void close()}>
              <span aria-hidden="true">←</span>Volver a Artículos
            </Button>
            <Text fontSize="lg" fontWeight="700" truncate minW="120px">{a?.name ?? ''}</Text>
            <SaveIndicator status={auto.status} onRetry={auto.retry} />
            <Box flex="1" />
            {a && !a.isActive && <Badge colorPalette="gray">Archivado</Badge>}
            {canView && <Button minH="44px" variant="outline" onClick={() => setPanel('audit')}>Bitácora</Button>}
          </HStack>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} display="flex" pb={3}>
          {!a || !form ? <Spinner m="auto" /> : (
            <SimpleGrid columns={{ base: 1, md: 2 }} gap={3} flex="1" minH={0} overflowY={{ base: 'auto', md: 'visible' }}>
              {/* Columna izquierda: lo que se edita. */}
              <Stack gap={3} borderWidth="1px" borderRadius="lg" p={3} minH={0} overflowY={{ md: 'auto' }} order={{ base: 2, md: 1 }}>
                {justCreated && <PacksBlock />}
                <Field label="Nombre" error={auto.invalid.name}>
                  <Input minH="48px" maxLength={60} disabled={!canConfigure} value={form.name}
                    aria-invalid={!!auto.invalid.name || auto.unsaved.includes('name')}
                    onChange={(e) => { setForm({ ...form, name: e.target.value }); auto.type('name', e.target.value); }}
                    onBlur={() => auto.commit('name', form.name)}
                    onKeyDown={(e) => { if (e.key === 'Enter') auto.commit('name', form.name); }} />
                </Field>
                <Field label="Categoría">
                  <Picker title="Categoría" placeholder="Sin categoría" clearable clearLabel="Sin categoría" disabled={!canConfigure}
                    value={form.categoryId} options={(cats.data ?? []).map((c) => ({ value: String(c.id), label: c.name }))}
                    onChange={(v) => { if (auto.busy('categoryId')) return; setForm({ ...form, categoryId: v }); auto.commit('categoryId', v); }}
                    onCreate={async (name) => {
                      try {
                        const c = await backofficeApi.createArticleCategory(name);
                        await qc.invalidateQueries({ queryKey: ['article-categories'] });
                        return { value: String(c.id), label: c.name };
                      } catch {
                        toaster.create({ title: 'No se pudo crear la categoría', type: 'error' });
                        return null;
                      }
                    }} />
                </Field>
                <Field label="Ubicación">
                  <Picker {...locations.pickerProps} disabled={!canConfigure} value={form.placeId} options={locations.options}
                    onChange={(v) => { if (auto.busy('placeId')) return; setForm({ ...form, placeId: v }); auto.commit('placeId', v); }} />
                </Field>
                <Stack gap={1}>
                  <QtyWithUnit label="Mínimo" kind={kind} value={form.min} keepAmount
                    onChange={(min) => { setForm({ ...form, min }); if (min.qty !== form.min.qty) auto.type('min', minValue(min)); }}
                    onCommit={() => auto.commit('min', minValue(form.min))} />
                  {auto.invalid.min && <Text fontSize="sm" color="red.fg">{auto.invalid.min}</Text>}
                </Stack>
                <HStack justify="space-between" minH="44px">
                  <Text>Se mide en <b>{KIND_WORD[kind]}</b>{a.baseUnitCode === 'pieza' ? '' : ` (${a.baseUnitCode})`}{Number(a.onHand) < 0 ? '' : ` · Hay ${formatBaseQty(a.onHand, a.baseUnitCode)}`}</Text>
                  {canConfigure && a.isActive && <Button minH="44px" variant="outline" onClick={() => setPanel('unit')}>Cambiar ›</Button>}
                </HStack>
                {/* Una existencia negativa no es un dato normal: dice qué hacer, en su propio renglón para
                    no partir el de arriba en media pantalla, y con texto además del rojo (spec 043, FR-018).
                    No se recalcula nada del pasado. */}
                {Number(a.onHand) < 0 && (
                  <HStack data-negative-stock gap={2} flexWrap="wrap" bg="red.subtle" borderRadius="md" px={3} py={2}>
                    <Text color="red.fg" fontWeight="700">Hay {formatBaseQty(a.onHand, a.baseUnitCode)}</Text>
                    <Text color="red.fg">Revisar con un conteo</Text>
                  </HStack>
                )}
                {!justCreated && <PacksBlock />}
                <Button minH="44px" variant="outline" justifyContent="flex-start" onClick={() => setPanel('recipe')}>
                  {a.isPrep ? 'Es un preparado · ver receta ›' : 'Se compra hecho ›'}
                  {a.compositionStatus === 'estimated' && <Badge ml={2} colorPalette="orange">por revisar</Badge>}
                </Button>
                {canConfigure && (
                  <Box pt={6}>
                    {a.isActive
                      ? <Button minH="44px" variant="outline" colorPalette="red" onClick={() => setPanel('archive')}>Archivar artículo</Button>
                      : <Button minH="44px" variant="outline" onClick={() => setPanel('archive')}>Reactivar</Button>}
                  </Box>
                )}
              </Stack>

              {/* Columna derecha: el costo y su historial. */}
              <Stack gap={2} borderWidth="1px" borderRadius="lg" p={3} minH={0} order={{ base: 1, md: 2 }}>
                <HStack gap={2} flexWrap="wrap">
                  <Text fontWeight="700">Costo</Text>
                  <Text fontSize="xl" fontWeight="800">{formatUnitCost(a.currentCost, a.baseUnitCode) ?? 'Sin costo'}</Text>
                  {trend != null && Number(trend) !== 0 && (
                    <Badge colorPalette={Number(trend) > 0 ? 'red' : 'green'} fontSize="sm">
                      {Number(trend) > 0 ? '▲ Sube' : '▼ Baja'} {Math.abs(Number(trend))} %
                    </Badge>
                  )}
                  <Box flex="1" />
                  {canConfigure && !a.isPrep && <Button minH="44px" variant="outline" onClick={() => setPanel('cost')}>Corregir costo</Button>}
                </HStack>
                {a.isPrep && <Text fontSize="sm" color="fg.muted">El costo de un preparado sale de su receta.</Text>}
                <CostTrend items={items} baseUnitCode={a.baseUnitCode} day={(s) => hora.soloFecha(s)} />
                <Text fontSize="xs" color="fg.muted" letterSpacing="wide" textTransform="uppercase">Compras y correcciones, la más reciente arriba</Text>
                <Box flex="1" minH={0} overflowY="auto">
                  {items.length === 0 && !history.isLoading && <Text color="fg.muted" py={2}>Todavía no hay compras de este artículo.</Text>}
                  {items.map((it) => (
                    <HStack key={`${it.kind}-${it.id}`} minH="48px" borderBottomWidth="1px" gap={3}>
                      <Text w="96px" fontSize="sm" color="fg.muted" flexShrink={0}>{hora.soloFecha(it.at)}</Text>
                      <Box flex="1" minW={0}>
                        {it.kind === 'purchase' ? (
                          <>
                            <Text fontSize="sm" truncate>{it.supplier ?? 'Compra'}</Text>
                            {it.presentation && it.packCost && (
                              <Text fontSize="xs" color="fg.muted">${Number(it.packCost).toFixed(2)} por {it.presentation} · {formatBaseQty(it.packQty ?? '0', a.baseUnitCode)}</Text>
                            )}
                          </>
                        ) : <Text fontSize="sm" color="orange.fg">Capturado a mano · {it.user ?? ''}</Text>}
                      </Box>
                      <Text fontWeight="700" flexShrink={0}>{formatUnitCost(it.unitCost, a.baseUnitCode)}</Text>
                    </HStack>
                  ))}
                  {history.hasNextPage && (
                    <Button mt={2} minH="44px" variant="outline" w="100%" loading={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>
                      Ver más
                    </Button>
                  )}
                </Box>
              </Stack>
            </SimpleGrid>
          )}
        </DrawerBody>
      </DrawerContent>

      {panel === 'audit' && <ArticleAuditSheet id={id} name={a?.name ?? ''} baseUnitCode={a?.baseUnitCode ?? 'pieza'} onClose={() => setPanel(null)} />}
      {panel === 'unit' && a && <UnitChangeSheet article={a} onClose={() => setPanel(null)} onDone={async () => { setPanel(null); await refresh(); }} />}
      {panel === 'archive' && a && <ArchiveSheet article={a} onClose={() => setPanel(null)} onDone={async () => { setPanel(null); await refresh(); }} />}
      {panel === 'recipe' && a && <CompositionSheet kind="ingredient" id={id} name={a.name} open onClose={() => setPanel(null)} />}
      {panel === 'packs' && a && (
        <PresentationsSheet isOpen startNew={editPack === undefined} editId={editPack} ingredient={{ id, name: a.name, baseUnitCode: a.baseUnitCode, baseUnitKind: a.baseUnitKind }}
          presentations={packs} units={units.data?.items ?? []}
          onClose={async () => { setPanel(null); setEditPack(undefined); await refresh(); }} />
      )}
      {panel === 'cost' && a && (
        <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) setPanel(null); }}>
          <DrawerBackdrop />
          <DrawerContent borderTopRadius="l3">
            <DrawerHeader><Text fontSize="lg" fontWeight="700">Corregir costo</Text></DrawerHeader>
            <DrawerBody pb={4}>
              <Stack gap={3}>
                <Text color="fg.muted">El costo nuevo se usa desde ahora. Lo ya vendido no cambia.</Text>
                <HStack gap={2}>
                  <Input minH="48px" inputMode="decimal" aria-label="Costo nuevo" placeholder="0.00" value={cost}
                    onChange={(e) => setCost(e.target.value.replace(',', '.'))} />
                  <Text flexShrink={0}>$ por {big?.label ?? 'pieza'}</Text>
                </HStack>
                <HStack gap={2} justify="flex-end">
                  <Button minH="48px" variant="outline" onClick={() => setPanel(null)}>Cancelar</Button>
                  <Button minH="48px" colorPalette="brand" loading={correct.isPending}
                    disabled={cost.trim() === '' || !(Number(cost) >= 0)} onClick={() => correct.mutate()}>Guardar costo</Button>
                </HStack>
              </Stack>
            </DrawerBody>
          </DrawerContent>
        </DrawerRoot>
      )}
      <ConfirmSheet isOpen={panel === 'discard'} title="¿Descartar cambios?" description="Lo que cambiaste en la ficha no se ha guardado."
        confirmLabel="Descartar" cancelLabel="Seguir editando" destructive onConfirm={onClose} onCancel={() => setPanel(null)} />
    </DrawerRoot>
  );

  function PacksBlock() {
    return (
      <Stack gap={2}>
        <HStack justify="space-between">
          <Text fontWeight="700">Empaques</Text>
          {canConfigure && <Button minH="44px" variant="outline" onClick={() => { setEditPack(undefined); setPanel('packs'); }}>+ Agregar empaque</Button>}
        </HStack>
        <Text fontSize="sm" color="fg.muted">Cómo lo compras, por ejemplo: {packExample(a?.baseUnitKind ?? 'pieza')}. Al recibirlo se cuenta en {a?.baseUnitCode === 'pieza' ? 'piezas' : a?.baseUnitCode}.</Text>
        {(a?.presentations ?? []).length === 0
          ? <Text fontSize="sm" color="fg.muted">Sin empaques.</Text>
          : (a?.presentations ?? []).map((p) => (
            <Button key={p.id} minH="48px" variant="ghost" justifyContent="space-between" borderBottomWidth="1px" borderRadius="0"
              disabled={!canConfigure} onClick={() => { setEditPack(p.id); setPanel('packs'); }}>
              <Text><b>{p.name}</b> · {formatBaseQty(p.qtyInBase, a!.baseUnitCode)}</Text>
              {canConfigure && <Text color="fg.muted">Editar ›</Text>}
            </Button>
          ))}
      </Stack>
    );
  }
}

function Field({ label, error, children }: { label: string; error?: string; children: React.ReactNode }) {
  return (
    <Stack gap={1}>
      <Text>{label}</Text>
      {children}
      {error && <Text fontSize="sm" color="red.fg">{error}</Text>}
    </Stack>
  );
}

// SaveIndicator es el único aviso del autoguardado: un punto mientras hay cambios sin mandar, después
// «Guardando…» y «Guardado ✓», o el fallo con «Reintentar».
function SaveIndicator({ status, onRetry }: { status: AutosaveStatus; onRetry: () => void }) {
  if (status === 'dirty') return <Box aria-label="Cambios sin guardar" title="Cambios sin guardar" w="10px" h="10px" borderRadius="full" bg="orange.solid" flexShrink={0} />;
  if (status === 'saving') return <HStack gap={2} color="fg.muted" flexShrink={0}><Spinner size="sm" /><Text fontSize="sm">Guardando…</Text></HStack>;
  if (status === 'saved') return <Text fontSize="sm" color="green.fg" flexShrink={0}>Guardado ✓</Text>;
  if (status === 'error') {
    return (
      <HStack gap={2} flexShrink={0}>
        <Text fontSize="sm" color="red.fg">No se pudo guardar</Text>
        <Button minH="44px" size="sm" variant="outline" colorPalette="red" onClick={onRetry}>Reintentar</Button>
      </HStack>
    );
  }
  return null;
}
