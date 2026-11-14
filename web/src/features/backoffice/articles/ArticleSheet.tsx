import { useCallback, useEffect, useRef, useState } from 'react';
import { medirAccion } from '../../../api/uso';
import { Badge, Box, Button, HStack, Input, SimpleGrid, Spinner, Stack, Text } from '@chakra-ui/react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader,
} from '../../../components/ui/drawer';
import { backofficeApi, type ArticleSheet as Sheet, type ArticleUpdate } from '../../../api/backoffice';
import { ApiError } from '../../../api/client';
import { InfoButton } from '../../../components/InfoButton';
import { Picker } from '../../../components/Picker';
import { AltNamesField } from '../../../components/AltNamesField';
import { altNameKey, withPendingAlt } from '../../../components/altNames';
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
import { MergeSheet } from './MergeSheet';
import { DensitySheet } from './DensitySheet';
import { densityWords } from '../expenseDraft';
import { presentationLabel } from '../../../shared/presentationLabel';
import { bigUnitOf, KIND_WORD, type Kind } from '../../../shared/articleUnits';
import { SeccionAlertas } from '../../../shared/alertas/SeccionAlertas';

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
  // onCount lleva al conteo de este artículo (spec 045); sin él, el aviso de negativo es solo texto.
  onCount?: (name: string) => void;
  // onOpenArticle abre otra ficha: la del insumo al que se unió éste (spec 056).
  onOpenArticle?: (id: number) => void;
}

interface Form {
  name: string;
  categoryId: string;
  placeId: string;
  min: { qty: string; big: boolean };
  alts: string[];
}

// Saved son los valores tal como se guardan: el mínimo en la unidad base, '' para vacío.
// alts va como JSON: la cola guarda textos, y la lista de otros nombres (spec 063) se manda completa.
type Saved = { name: string; categoryId: string; placeId: string; min: string; alts: string };

function savedOf(a: Sheet): Saved {
  return {
    name: a.name,
    categoryId: a.categoryId == null ? '' : String(a.categoryId),
    placeId: a.countPlaceId == null ? '' : String(a.countPlaceId),
    min: a.minStock == null ? '' : String(Number(a.minStock)),
    alts: JSON.stringify(a.altNames ?? []),
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
    alts: a.altNames ?? [],
  };
}

export function ArticleSheet({ id, justCreated, onClose, onCount, onOpenArticle }: Props) {
  const qc = useQueryClient();
  const user = useSessionStore((s) => s.user);
  const canConfigure = can('inventory.configure', user);
  const canReceive = can('inventory.receive', user);
  const canView = can('inventory.view', user);
  const canCount = can('inventory.count', user);
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
  const [panel, setPanel] = useState<'audit' | 'archive' | 'packs' | 'recipe' | 'cost' | 'discard' | 'unit' | 'merge' | 'density' | null>(null)
  const [editPack, setEditPack] = useState<number | undefined>(undefined);
  // La clase a la que se quería pasar cuando la densidad se pidió desde el cambio de unidad: al
  // guardarla se vuelve ahí con ese destino elegido, en vez de cerrar las dos hojas.
  const [unitTarget, setUnitTarget] = useState<Kind | null>(null);
  const [cost, setCost] = useState('');
  // Otros nombres (spec 063): lo escrito sin agregar y el choque que respondió el servidor.
  const [altDraft, setAltDraft] = useState('');
  const [altError, setAltError] = useState('');
  const altRejected = useRef(false);
  const similarAlts = useCallback((name: string) => backofficeApi.similarArticleAltNames(id, name), [id]);

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
    initial: a ? savedOf(a) : { name: '', categoryId: '', placeId: '', min: '', alts: '[]' },
    version: a?.updatedAt ?? '',
    // Manda la ficha completa: lo que cambió más lo último guardado de lo demás. La API anterior, que
    // atiende durante una subida, toma un campo ausente como vacío y lo borraría. La nueva compara
    // contra lo que había y deja en la bitácora solo lo que cambió.
    save: async (patch, version, saved) => {
      // Los otros nombres van por su propia ruta (spec 063); devuelve la versión nueva de la ficha.
      if (patch.alts !== undefined) {
        const list = JSON.parse(patch.alts) as string[];
        const r = await backofficeApi.setArticleAltNames(id, list, version);
        setAltDraft((d) => (list.some((n) => altNameKey(n) === altNameKey(d)) ? '' : d));
        void qc.invalidateQueries({ queryKey: ['articles'] });
        void qc.invalidateQueries({ queryKey: ['article', id] });
        void qc.invalidateQueries({ queryKey: ['article-audit', id] });
        return { updatedAt: r.updatedAt };
      }
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
      if (k === 'alts') {
        altRejected.current = true;
        // Con otro guardado de la lista en cola, ése trae lo último: regresar a lo guardado borraría de
        // la pantalla lo que la cola está por guardar (revisión del front).
        if (auto.busy('alts')) { setAltError(e instanceof ApiError && e.status < 500 ? e.message : 'No se pudo guardar'); return; }
        setForm((f) => f && { ...f, alts: JSON.parse(saved) as string[] });
        setAltError(e instanceof ApiError && e.status < 500 ? e.message : 'No se pudo guardar');
        return;
      }
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
        alts: keep.has('alts') ? prev.alts : next.alts,
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
  const changeAlts = (alts: string[]) => {
    setAltError('');
    setForm((f) => f && { ...f, alts });
    auto.commit('alts', JSON.stringify(alts));
  };
  const [leaving, setLeaving] = useState(false);
  // settle saca el foco del campo (eso lo manda a guardar) y espera a que la cola termine.
  const settle = async () => {
    const el = document.activeElement;
    if (el instanceof HTMLElement && el.tagName === 'INPUT') el.blur();
    setLeaving(true);
    const allSaved = await auto.whenIdle();
    setLeaving(false);
    return allSaved;
  };
  // Lo escrito sin «Agregar» se guarda al salir de la ficha, por donde sea; si choca, la ficha se queda
  // con el aviso (A1). Devuelve si se puede salir.
  const savePending = async () => {
    altRejected.current = false;
    if (form) {
      const pending = withPendingAlt(form.alts, altDraft, form.name);
      if (pending !== form.alts) changeAlts(pending);
    }
    const allSaved = await settle();
    return { allSaved, rejected: altRejected.current };
  };
  const close = async () => {
    if (leaving) return;
    const { allSaved, rejected } = await savePending();
    if (allSaved) {
      if (!rejected) onClose();
    } else setPanel('discard');
  };
  // Ir al conteo no pregunta si descartar: si algo no se guardó, se queda con el error a la vista (B12).
  const goCount = async () => {
    if (leaving) return;
    // El nombre de la ficha, ya guardado: si se acaba de renombrar, el conteo lo busca con el nuevo.
    if (!form) return;
    const { allSaved, rejected } = await savePending();
    if (allSaved && !rejected) onCount?.(form.name.trim());
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
                <AltNamesField kind="article" ownName={form.name} value={form.alts} onChange={changeAlts} disabled={!canConfigure}
                  draft={altDraft} onDraftChange={(d) => { setAltDraft(d); setAltError(''); }} similar={similarAlts} error={altError} />
                {/* El mínimo vive en «Alertas» (spec 084, maqueta M1-B): un renglón con el estado y su hoja. */}
                <SeccionAlertas itemType="ingrediente" id={id} name={a.name} kind={kind} canConfigure={canConfigure} />
                <HStack justify="space-between" minH="44px">
                  <Text>Se mide en <b>{KIND_WORD[kind]}</b>{a.baseUnitCode === 'pieza' ? '' : ` (${a.baseUnitCode})`}{Number(a.onHand) < 0 ? '' : ` · Hay ${formatBaseQty(a.onHand, a.baseUnitCode)}`}</Text>
                  {canConfigure && a.isActive && <Button minH="44px" variant="outline" onClick={() => setPanel('unit')}>Cambiar ›</Button>}
                </HStack>
                {(kind === 'masa' || kind === 'volumen') && (
                  <DensityRow density={a.density ?? null} day={(s) => hora.soloFecha(s)}
                    canEdit={canConfigure && a.isActive} onEdit={() => setPanel('density')} />
                )}
                {/* Una existencia negativa no es un dato normal: dice qué hacer, en su propio renglón para
                    no partir el de arriba en media pantalla, y con texto además del rojo (spec 043, FR-018).
                    No se recalcula nada del pasado. */}
                {Number(a.onHand) < 0 && (
                  <HStack data-negative-stock gap={2} bg="red.subtle" borderRadius="md" pl={3} minH="44px">
                    <Text flex="1" color="fg.error" fontWeight="700">Hay {formatBaseQty(a.onHand, a.baseUnitCode)}</Text>
                    {canCount && onCount ? (
                      <Button size="sm" minH="44px" variant="outline" colorPalette="brand" bg="bg" flexShrink={0}
                        loading={leaving} onClick={() => void goCount()}>Revisar con un conteo ›</Button>
                    ) : <Text color="fg.error" pr={3}>Revisar con un conteo</Text>}
                  </HStack>
                )}
                {!justCreated && <PacksBlock />}
                {/* El estado en texto y la acción en su botón, como «Se mide en … · Cambiar ›» (#103): en un
                    solo botón se leía como un dato. */}
                <HStack justify="space-between" minH="44px" data-recipe-row>
                  <Text>Receta: <b>{a.isPrep ? 'es un preparado' : 'se compra hecho'}</b>
                    {a.compositionStatus === 'estimated' && <Badge ml={2} colorPalette="orange">por revisar</Badge>}</Text>
                  <Button minH="44px" variant="outline" onClick={() => setPanel('recipe')}>{a.isPrep ? 'Ver receta ›' : 'Agregar receta ›'}</Button>
                </HStack>
                {a.mergedInto ? (
                  // Unido a otro (spec 056): no se reactiva; lleva al que se quedó.
                  <HStack gap={2} bg="bg.muted" borderRadius="md" pl={3} minH="44px" data-merged-into>
                    <Text flex="1">Unido a <b>{a.mergedInto.name}</b> el {hora.soloFecha(a.mergedInto.at)}</Text>
                    {onOpenArticle && (
                      <Button minH="44px" variant="outline" bg="bg" aria-label={`Abrir ${a.mergedInto.name}`}
                        onClick={() => onOpenArticle(a.mergedInto!.id)}>Abrir ›</Button>
                    )}
                  </HStack>
                ) : canConfigure && (
                  <HStack pt={6} gap={3} flexWrap="wrap">
                    {a.isActive && !a.isPrep && <Button minH="44px" variant="outline" onClick={() => setPanel('merge')}>Unir otros aquí</Button>}
                    <Box flex="1" />
                    {a.isActive
                      ? <Button minH="44px" variant="outline" colorPalette="red" onClick={() => setPanel('archive')}>Archivar artículo</Button>
                      : <Button minH="44px" variant="outline" onClick={() => setPanel('archive')}>Reactivar</Button>}
                  </HStack>
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
                            <Text fontSize="sm" truncate>{it.supplier ?? 'Compra'}{it.fromName ? ` · de ${it.fromName}` : ''}</Text>
                            {it.presentation && it.packCost && (
                              <Text fontSize="xs" color="fg.muted">${Number(it.packCost).toFixed(2)} por {it.presentation}{it.brand ? ` · ${it.brand}` : ''} · {formatBaseQty(it.packQty ?? '0', a.baseUnitCode)}</Text>
                            )}
                          </>
                        ) : <Text fontSize="sm" color="orange.fg">Capturado a mano · {it.user ?? ''}{it.fromName ? ` · de ${it.fromName}` : ''}</Text>}
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
      {panel === 'unit' && a && (
        <UnitChangeSheet article={a} initialTo={unitTarget ?? undefined}
          onClose={() => { setUnitTarget(null); setPanel(null); }}
          onDone={async () => { setUnitTarget(null); setPanel(null); await refresh(); }}
          onNeedDensity={(to) => { setUnitTarget(to); setPanel('density'); }} />
      )}
      {panel === 'density' && a && (
        <DensitySheet article={a} onClose={() => { setUnitTarget(null); setPanel(null); }}
          onDone={async () => {
            // Primero la ficha con la densidad nueva: el cambio de unidad la necesita para dejar elegir el destino.
            setPanel(null); await refresh();
            if (unitTarget) setPanel('unit');
          }} />
      )}
      {panel === 'merge' && a && (
        <MergeSheet target={a} onClose={() => setPanel(null)} onDone={async () => { setPanel(null); await refresh(); }} />
      )}
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
              <Text><b>{presentationLabel(p)}</b> · {formatBaseQty(p.qtyInBase, a!.baseUnitCode)}</Text>
              {canConfigure && <Text color="fg.muted">Editar ›</Text>}
            </Button>
          ))}
      </Stack>
    );
  }
}

const METHOD_WORDS: Record<string, string> = { weighing: 'por pesada', label: 'de la etiqueta', manual: 'escrita' };

// DensityRow es la densidad en la ficha (spec 058): opcional, pegada a «Se mide en». El (i) despliega
// debajo para no mover el botón.
function DensityRow({ density, day, canEdit, onEdit }: {
  density: Sheet['density'] | null; day: (s: string) => string; canEdit: boolean; onEdit: () => void;
}) {
  const [help, setHelp] = useState(false);
  return (
    <Stack gap={1} data-density-row>
      <HStack justify="space-between" minH="44px" gap={2}>
        <HStack gap={1} minW={0} flexWrap="wrap">
          <Text>Densidad</Text>
          <Badge colorPalette="gray">Opcional</Badge>
          <InfoButton label="¿Para qué sirve la densidad?" open={help} onToggle={() => setHelp((h) => !h)} />
          <Text color={density ? undefined : 'fg.muted'}>
            {density
              ? `${densityWords(Number(density.value))} gramos por ml · ${METHOD_WORDS[density.method] ?? ''} · ${density.setByName}, ${day(density.setAt)}`
              : 'Sin capturar'}
          </Text>
        </HStack>
        {canEdit && (
          <Button minH="44px" variant="outline" flexShrink={0} aria-label={density ? 'Cambiar la densidad' : 'Capturar la densidad'} onClick={onEdit}>
            {density ? 'Cambiar' : 'Capturar'}
          </Button>
        )}
      </HStack>
      {help && (
        <Text fontSize="sm" color="fg.muted" p={2} borderRadius="md" bg="bg.subtle">
          Sirve para comprar en peso algo que guardas en líquido, o al revés. Por ejemplo, el galón dice 4.3 kg y lo guardas en ml;
          con la densidad 1.10 entra solo como 4,300 g ÷ 1.10 = 3,909 ml. Sin ella todo funciona igual que hoy.
        </Text>
      )}
    </Stack>
  );
}

function Field({ label, error, children }: { label: string; error?: string; children: React.ReactNode }) {
  return (
    <Stack gap={1}>
      <Text>{label}</Text>
      {children}
      {error && <Text fontSize="sm" color="fg.error">{error}</Text>}
    </Stack>
  );
}

// SaveIndicator es el único aviso del autoguardado: un punto mientras hay cambios sin mandar, después
// «Guardando…» y «Guardado ✓», o el fallo con «Reintentar».
function SaveIndicator({ status, onRetry }: { status: AutosaveStatus; onRetry: () => void }) {
  if (status === 'dirty') return <Box aria-label="Cambios sin guardar" title="Cambios sin guardar" w="10px" h="10px" borderRadius="full" bg="orange.solid" flexShrink={0} />;
  if (status === 'saving') return <HStack gap={2} color="fg.muted" flexShrink={0}><Spinner size="sm" /><Text fontSize="sm">Guardando…</Text></HStack>;
  if (status === 'saved') return <Text fontSize="sm" color="fg.success" flexShrink={0}>Guardado ✓</Text>;
  if (status === 'error') {
    return (
      <HStack gap={2} flexShrink={0}>
        <Text fontSize="sm" color="fg.error">No se pudo guardar</Text>
        <Button minH="44px" size="sm" variant="outline" colorPalette="brand" onClick={onRetry}>Reintentar</Button>
      </HStack>
    );
  }
  return null;
}
