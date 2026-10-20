import { useEffect, useMemo, useState } from 'react';
import { Badge, Box, Button, HStack, Input, SimpleGrid, Spinner, Stack, Text } from '@chakra-ui/react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader,
} from '../../../components/ui/drawer';
import { backofficeApi, type ArticleSheet as Sheet } from '../../../api/backoffice';
import { ApiError } from '../../../api/client';
import { Picker } from '../../../components/Picker';
import { ConfirmSheet } from '../../../components/ConfirmSheet';
import { toaster } from '../../../components/ui/toaster';
import { can } from '../../../app/permissions';
import { useSessionStore } from '../../../stores/session';
import { useHoraDelNegocio } from '../../../hooks/useHoraDelNegocio';
import { formatBaseQty, formatUnitCost, plainQty } from '../../../shared/units';
import { CompositionSheet } from '../../../shared/CompositionSheet';
import { PresentationsSheet } from '../PresentationsSheet';
import { CostTrend } from './CostTrend';
import { ArticleAuditSheet } from './ArticleAuditSheet';
import { ArchiveSheet } from './ArchiveSheet';
import { QtyWithUnit } from './qty';
import { baseQty, bigUnitOf, KIND_WORD, type Kind } from './articleUnits';

// La ficha de un artículo (spec 042, maqueta «Ficha del artículo», opción A: dos columnas). A la
// izquierda lo que se edita y los empaques; a la derecha el costo, su tendencia y el historial. De pie
// (menos de 800 px de ancho) es una sola columna con el costo arriba (revisión de tableta, M2).

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
  const places = useQuery({ queryKey: ['count-places'], queryFn: backofficeApi.countPlaces });
  const units = useQuery({ queryKey: ['units'], queryFn: backofficeApi.units, enabled: canConfigure });
  const allPres = useQuery({ queryKey: ['presentations'], queryFn: backofficeApi.presentations, enabled: canReceive });
  const history = useInfiniteQuery({
    queryKey: ['article-cost-history', id],
    queryFn: ({ pageParam }) => backofficeApi.articleCostHistory(id, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (p) => p.next ?? undefined,
  });
  const [form, setForm] = useState<Form | null>(null);
  const [panel, setPanel] = useState<'audit' | 'archive' | 'packs' | 'recipe' | 'cost' | 'discard' | null>(null);
  const [cost, setCost] = useState('');

  useEffect(() => {
    if (art.data && form === null) setForm(formOf(art.data));
  }, [art.data, form]);

  const a = art.data;
  const kind = (a?.baseUnitKind ?? 'pieza') as Kind;
  const big = bigUnitOf(kind);
  const dirty = useMemo(() => !!a && !!form && JSON.stringify(form) !== JSON.stringify(formOf(a)), [a, form]);

  const refresh = async () => {
    setForm(null);
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['article', id] }),
      qc.invalidateQueries({ queryKey: ['articles'] }),
      qc.invalidateQueries({ queryKey: ['article-cost-history', id] }),
      qc.invalidateQueries({ queryKey: ['article-audit', id] }),
    ]);
  };
  const save = useMutation({
    mutationFn: () => backofficeApi.updateArticle(id, {
      expectedUpdatedAt: a!.updatedAt, name: form!.name,
      categoryId: form!.categoryId ? Number(form!.categoryId) : null,
      placeId: form!.placeId ? Number(form!.placeId) : null,
      minStock: baseQty(form!.min.qty, kind, form!.min.big) ?? null,
    }),
    onSuccess: async () => { await refresh(); toaster.create({ title: 'Guardado', type: 'success' }); },
    onError: async (e) => {
      if (e instanceof ApiError && e.code === 'STALE') {
        toaster.create({ title: 'Cambió mientras la veías', description: 'Se volvió a cargar con lo último.', type: 'warning' });
        await refresh();
        return;
      }
      toaster.create({ title: e instanceof ApiError && e.status !== 500 ? e.message : 'No se pudo guardar', type: 'error' });
    },
  });
  const correct = useMutation({
    mutationFn: () => backofficeApi.correctArticleCost(id, String(Number(cost) / (big?.factor ?? 1))),
    onSuccess: async () => { setPanel(null); setCost(''); await refresh(); toaster.create({ title: 'Costo corregido', type: 'success' }); },
    onError: (e) => toaster.create({ title: e instanceof ApiError && e.status !== 500 ? e.message : 'No se pudo corregir el costo', type: 'error' }),
  });

  const close = () => (dirty ? setPanel('discard') : onClose());
  const items = history.data?.pages.flatMap((p) => p.items ?? []) ?? [];
  const trend = history.data?.pages[0]?.trendPct;
  const packs = (allPres.data ?? []).filter((p) => p.ingredientId === id);

  return (
    <DrawerRoot open placement="bottom" size="full" onOpenChange={(e) => { if (!e.open) close(); }}>
      <DrawerBackdrop />
      <DrawerContent display="flex" flexDirection="column">
        <DrawerHeader py={2}>
          <HStack gap={2}>
            <Button minH="44px" variant="ghost" onClick={close}>‹ Artículos</Button>
            <Text flex="1" fontSize="lg" fontWeight="700" truncate>{a?.name ?? ''}</Text>
            {a && !a.isActive && <Badge colorPalette="gray">Archivado</Badge>}
            {canView && <Button minH="44px" variant="outline" onClick={() => setPanel('audit')}>Bitácora</Button>}
            {canConfigure && (
              <Button minH="44px" colorPalette="brand" disabled={!dirty} loading={save.isPending} onClick={() => save.mutate()}>Guardar</Button>
            )}
          </HStack>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} display="flex" pb={3}>
          {!a || !form ? <Spinner m="auto" /> : (
            <SimpleGrid columns={{ base: 1, md: 2 }} gap={3} flex="1" minH={0} overflowY={{ base: 'auto', md: 'visible' }}>
              {/* Columna izquierda: lo que se edita. */}
              <Stack gap={3} borderWidth="1px" borderRadius="lg" p={3} minH={0} overflowY={{ md: 'auto' }} order={{ base: 2, md: 1 }}>
                {justCreated && <PacksBlock />}
                <Field label="Nombre">
                  <Input minH="48px" maxLength={60} disabled={!canConfigure} value={form.name}
                    onChange={(e) => setForm({ ...form, name: e.target.value })} />
                </Field>
                <Field label="Categoría">
                  <Picker title="Categoría" placeholder="Sin categoría" clearable clearLabel="Sin categoría" disabled={!canConfigure}
                    value={form.categoryId} options={(cats.data ?? []).map((c) => ({ value: String(c.id), label: c.name }))}
                    onChange={(v) => setForm({ ...form, categoryId: v })}
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
                <Field label="Lugar">
                  <Picker title="Lugar" placeholder="Sin lugar" clearable clearLabel="Sin lugar" disabled={!canConfigure}
                    value={form.placeId} options={(places.data ?? []).map((p) => ({ value: String(p.id), label: p.name }))}
                    onChange={(v) => setForm({ ...form, placeId: v })} />
                </Field>
                <QtyWithUnit label="Mínimo" kind={kind} value={form.min} onChange={(min) => setForm({ ...form, min })} />
                <HStack justify="space-between" minH="44px">
                  <Text>Se mide en <b>{KIND_WORD[kind]}</b> ({a.baseUnitCode === 'pieza' ? 'piezas' : a.baseUnitCode})</Text>
                </HStack>
                {!justCreated && <PacksBlock />}
                <Button minH="44px" variant="outline" justifyContent="flex-start" onClick={() => setPanel('recipe')}>
                  {a.isPrep ? 'Es un preparado · ver receta ›' : 'Se compra hecho ›'}
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
                        ) : <Text fontSize="sm" color="orange.fg">Corrección a mano · {it.user ?? ''}</Text>}
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

      {panel === 'audit' && <ArticleAuditSheet id={id} name={a?.name ?? ''} onClose={() => setPanel(null)} />}
      {panel === 'archive' && a && <ArchiveSheet article={a} onClose={() => setPanel(null)} onDone={async () => { setPanel(null); await refresh(); }} />}
      {panel === 'recipe' && a && <CompositionSheet kind="ingredient" id={id} name={a.name} open onClose={() => setPanel(null)} />}
      {panel === 'packs' && a && (
        <PresentationsSheet isOpen ingredient={{ id, name: a.name, baseUnitCode: a.baseUnitCode, baseUnitKind: a.baseUnitKind }}
          presentations={packs} units={units.data?.items ?? []}
          onClose={async () => { setPanel(null); await refresh(); }} />
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
          {canConfigure && <Button minH="44px" variant="outline" onClick={() => setPanel('packs')}>+ Agregar empaque</Button>}
        </HStack>
        <Text fontSize="sm" color="fg.muted">Cómo lo compras, por ejemplo: bolsa de 5 kg. Al recibirlo se cuenta en {a?.baseUnitCode === 'pieza' ? 'piezas' : a?.baseUnitCode}.</Text>
        {(a?.presentations ?? []).length === 0
          ? <Text fontSize="sm" color="fg.muted">Sin empaques.</Text>
          : (a?.presentations ?? []).map((p) => (
            <HStack key={p.id} minH="48px" borderBottomWidth="1px" justify="space-between">
              <Text><b>{p.name}</b> · {formatBaseQty(p.qtyInBase, a!.baseUnitCode)}</Text>
              <Text color="fg.muted">= {plainQty(p.qtyInBase)} {a!.baseUnitCode === 'pieza' ? 'piezas' : a!.baseUnitCode}</Text>
            </HStack>
          ))}
      </Stack>
    );
  }
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Stack gap={1}>
      <Text>{label}</Text>
      {children}
    </Stack>
  );
}
