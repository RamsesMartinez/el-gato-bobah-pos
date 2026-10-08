import { useEffect, useMemo, useState } from 'react';
import { Badge, Box, Button, HStack, Input, Progress, Text, VStack } from '@chakra-ui/react';
import { LuChevronRight, LuSearch } from 'react-icons/lu';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { adminApi, type CompositionKind, type RecipeKind, type RecipeRow, type RecipeStatus } from '../../api/admin';
import { Page } from '../../components/Page';
import { Picker } from '../../components/Picker';
import { toaster } from '../../components/ui/toaster';
import { CompositionSheet } from '../../shared/CompositionSheet';

// Menú › Recetas: la lista de trabajo para dejar dicho qué gasta cada producto, extra y
// preparado. Lo más vendido primero, porque capturar esas recetas cubre casi todo lo que sale del
// almacén; al guardar una se abre sola la siguiente.
const PAGE = 25;

const KINDS: { k: RecipeKind; label: string; search: string }[] = [
  { k: 'product', label: 'Productos', search: 'Buscar producto o insumo…' },
  { k: 'extra', label: 'Extras', search: 'Buscar extra, grupo o insumo…' },
  { k: 'prep', label: 'Preparados', search: 'Buscar preparado o insumo…' },
];
const STATUSES: { s: RecipeStatus; label: string }[] = [
  { s: 'pending', label: 'Pendientes' }, { s: 'review', label: 'Por revisar' }, { s: 'done', label: 'Listas' },
];
const sheetKind: Record<RecipeKind, CompositionKind> = { product: 'product', extra: 'option', prep: 'ingredient' };

function summaryOf(r: RecipeRow): string {
  if (r.mode === 'own') return 'Se descuenta solo';
  if (r.mode === 'combo') return r.summary ? `Combo: ${r.summary}` : 'Combo';
  if (r.mode === 'product') return `Es: ${r.summary}`;
  if (r.mode === 'items') return r.summary + (r.lines > 3 ? ` · +${r.lines - 3} más` : '');
  return r.status === 'done' ? 'No gasta insumos' : 'Falta la receta';
}

export function RecipesPage() {
  const qc = useQueryClient();
  const [kind, setKind] = useState<RecipeKind>('product');
  const [status, setStatus] = useState<RecipeStatus>('pending');
  const [sort, setSort] = useState<'sales' | 'az'>('sales');
  const [search, setSearch] = useState('');
  const [q, setQ] = useState(''); // con retraso: una petición por pausa de tecleo
  const [category, setCategory] = useState('');
  const [open, setOpen] = useState<RecipeRow | null>(null);

  useEffect(() => { const t = setTimeout(() => setQ(search.trim()), 300); return () => clearTimeout(t); }, [search]);

  const list = useInfiniteQuery({
    queryKey: ['admin', 'recipes', { kind, status, sort, q, category }],
    queryFn: ({ pageParam }) => adminApi.recipes({ kind, status, sort, q, category: category ? Number(category) : undefined, limit: PAGE, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((n, p) => n + (p.items ?? []).length, 0);
      return loaded < last.total ? loaded : undefined;
    },
  });
  const pages = useMemo(() => list.data?.pages ?? [], [list.data]);
  const rows = useMemo(() => pages.flatMap((p) => p.items ?? []), [pages]);
  const first = pages[0];
  const total = first?.total ?? 0;
  const counts = first?.counts ?? { pending: 0, review: 0, done: 0 };
  const totals = first?.totals;
  const kindTotals = totals?.[kind] ?? { pending: 0, review: 0, done: 0 };
  const all = kindTotals.pending + kindTotals.review + kindTotals.done;

  const categories = useQuery({ queryKey: ['admin', 'categories'], queryFn: adminApi.categories, enabled: kind === 'product' });
  const groups = useQuery({ queryKey: ['admin', 'groups', 'all-active'], queryFn: () => adminApi.groups({ status: 'act', limit: 0 }), enabled: kind === 'extra' });
  const catOptions = kind === 'product'
    ? (categories.data?.items ?? []).map((c) => ({ value: String(c.id), label: c.name }))
    : (groups.data?.items ?? []).map((g) => ({ value: String(g.id), label: g.name }));

  const confirmVisible = useMutation({
    mutationFn: () => adminApi.confirmRecipes(kind, rows.map((r) => r.id)),
    onSuccess: (r) => {
      qc.invalidateQueries({ queryKey: ['admin'] });
      toaster.create({ title: `Confirmadas ${r.confirmed} recetas`, type: 'success' });
    },
    onError: (e) => toaster.create({
      title: 'No se confirmaron',
      description: e instanceof ApiError ? e.message : 'No hay conexión con el servidor. Intenta de nuevo.',
      type: 'error',
    }),
  });

  // Abre en el primer estado con recetas: Preparados no tiene «Pendientes» y abría en una lista vacía
  // sin ningún botón marcado.
  const pickKind = (k: RecipeKind) => {
    const t = totals?.[k];
    const firstWithRecipes = STATUSES.find((x) => !(k === 'prep' && x.s === 'pending') && (t?.[x.s] ?? 0) > 0)?.s;
    setKind(k); setCategory(''); setSearch(''); setQ('');
    setStatus(firstWithRecipes ?? (k === 'prep' ? 'review' : 'pending'));
  };
  // Lo buscado que sí está, pero en otro estado: decir «nada coincide» con «Por revisar · 1» a la
  // vista es mentir.
  const elsewhere = q ? STATUSES.filter((x) => x.s !== status && counts[x.s] > 0) : [];
  // La siguiente de la lista que se ve, en su orden: así se avanza sin regresar a la lista.
  const openNext = () => {
    if (!open) return;
    const i = rows.findIndex((r) => r.id === open.id);
    const next = rows.slice(i + 1).find((r) => r.status !== 'done') ?? rows.slice(0, Math.max(i, 0)).find((r) => r.status !== 'done');
    setOpen(status === 'done' ? null : next ?? null);
  };
  const emptyTitle = q && elsewhere.length ? `Aquí no hay «${q}», pero sí en otro estado`
    : q ? `Nada coincide con «${q}»`
    : all > 0 && kindTotals.done === all ? 'Todas las recetas de esta lista están listas'
      : status === 'pending' ? 'No hay pendientes aquí' : status === 'review' ? 'No hay nada por revisar' : 'Todavía no hay recetas listas';

  return (
    <Page fill>
      <VStack align="stretch" gap={2} h="100%" minH={0}>
        <HStack gap={3} wrap="wrap">
          <HStack role="tablist" aria-label="Qué revisar" gap={1} bg="bg.muted" p={1} borderRadius="lg">
            {KINDS.map((k) => {
              const t = totals?.[k.k];
              const left = t ? t.pending + t.review : 0;
              return (
                <Button key={k.k} role="tab" aria-selected={kind === k.k} minH="44px" variant={kind === k.k ? 'solid' : 'ghost'}
                  colorPalette={kind === k.k ? undefined : 'gray'} onClick={() => pickKind(k.k)}>
                  {k.label}{left > 0 && <Badge colorPalette="orange" borderRadius="full">{left}</Badge>}
                </Button>
              );
            })}
          </HStack>
          <Box flex="1" />
          <VStack align="stretch" gap={1} w="230px">
            <HStack justify="space-between" fontSize="sm"><Text color="fg.muted">Recetas listas</Text><Text fontWeight="700">{kindTotals.done} de {all}</Text></HStack>
            <Progress.Root value={all ? (kindTotals.done / all) * 100 : 0} colorPalette="green" size="sm"><Progress.Track><Progress.Range /></Progress.Track></Progress.Root>
          </VStack>
        </HStack>

        <HStack gap={2}>
          <Box position="relative" flex="1">
            <Box position="absolute" left={3} top="50%" transform="translateY(-50%)" color="fg.muted" pointerEvents="none"><LuSearch /></Box>
            <Input aria-label="Buscar" placeholder={KINDS.find((k) => k.k === kind)?.search} value={search} onChange={(e) => setSearch(e.target.value)} minH="44px" ps={10} />
          </Box>
          {kind !== 'prep' && (
            <Box w="210px">
              <Picker value={category} onChange={(v) => setCategory(v)} options={catOptions} clearable
                clearLabel={kind === 'extra' ? 'Todos los grupos' : 'Todas las categorías'}
                placeholder={kind === 'extra' ? 'Grupo' : 'Categoría'} title={kind === 'extra' ? 'Grupo' : 'Categoría'} />
            </Box>
          )}
          {kind !== 'prep' && (
            <Button minH="44px" variant="outline" onClick={() => setSort((s) => (s === 'sales' ? 'az' : 'sales'))}>
              {sort === 'sales' ? 'Más vendidos primero' : 'De la A a la Z'}
            </Button>
          )}
        </HStack>

        <HStack gap={2} wrap="wrap">
          {STATUSES.filter((x) => !(kind === 'prep' && x.s === 'pending')).map((x) => (
            <Button key={x.s} minH="44px" borderRadius="full" variant={status === x.s ? 'solid' : 'outline'}
              colorPalette={status === x.s ? undefined : 'gray'} onClick={() => setStatus(x.s)}>
              {x.label} · {counts[x.s]}
            </Button>
          ))}
          <Box flex="1" />
          {status === 'review' && rows.length > 0 && (
            <Button minH="44px" variant="outline" colorPalette="orange" loading={confirmVisible.isPending} onClick={() => confirmVisible.mutate()}>
              Confirmar las {rows.length} de esta lista
            </Button>
          )}
        </HStack>
        {q && rows.length > 0 && (
          <Text fontSize="sm" color="blue.800" bg="blue.50" borderWidth="1px" borderColor="blue.200" borderRadius="md" px={3} py={1}>
            También se muestran las recetas que llevan «{q}».
          </Text>
        )}

        <Box flex="1" minH={0} overflowY="auto" borderWidth="1px" borderRadius="xl">
          {rows.map((r) => (
            <Button key={r.id} variant="ghost" w="100%" h="auto" minH="58px" px={4} py={2} gap={3} borderRadius={0}
              borderBottomWidth="1px" borderColor="border.muted" justifyContent="start" fontWeight="normal" onClick={() => setOpen(r)}>
              <StatusIcon status={r.status} />
              <VStack align="start" gap={0} flex="1" minW={0}>
                <Text fontWeight="600" truncate maxW="100%">{r.name}{kind === 'extra' && r.group && <Text as="span" fontWeight="normal" color="fg.muted" fontSize="sm">  ·  {r.group}</Text>}</Text>
                <Text fontSize="sm" color={r.status === 'pending' ? 'fg.subtle' : 'fg.muted'} fontStyle={r.status === 'pending' ? 'italic' : undefined} truncate maxW="100%">{summaryOf(r)}</Text>
              </VStack>
              {kind !== 'prep' && <Text fontSize="sm" color="fg.muted" w="80px" textAlign="end">{Number(r.soldPerMonth) > 0 ? `${Number(r.soldPerMonth)} al mes` : ''}</Text>}
              <Text fontSize="sm" fontWeight="600" w="88px" textAlign="end" color={r.status === 'pending' ? 'fg.muted' : r.status === 'review' ? 'orange.700' : 'green.700'}>
                {r.status === 'pending' ? 'Pendiente' : r.status === 'review' ? 'Por revisar' : 'Lista'}
              </Text>
              <LuChevronRight />
            </Button>
          ))}
          {list.hasNextPage && (
            <HStack justify="center" py={3}>
              <Button minH="44px" variant="outline" loading={list.isFetchingNextPage} onClick={() => list.fetchNextPage()}>
                Ver {PAGE} más · quedan {total - rows.length}
              </Button>
            </HStack>
          )}
          {!list.isLoading && rows.length === 0 && (
            <VStack py={12} px={6} gap={2} textAlign="center">
              <Text fontWeight="600" fontSize="lg">{emptyTitle}</Text>
              {q && !elsewhere.length && <Text color="fg.muted">Busca por nombre o por un insumo, como «tapioca».</Text>}
              {elsewhere.map((x) => (
                <Button key={x.s} minH="44px" colorPalette="orange" onClick={() => setStatus(x.s)}>Ver en {x.label} · {counts[x.s]}</Button>
              ))}
              {(q || category) && <Button minH="44px" variant="outline" onClick={() => { setSearch(''); setQ(''); setCategory(''); }}>Quitar la búsqueda</Button>}
            </VStack>
          )}
        </Box>
      </VStack>

      {open && (
        <CompositionSheet key={open.id} kind={sheetKind[kind]} id={open.id} name={open.name}
          group={kind === 'extra' ? open.group : undefined} open
          pendingLeft={status === 'done' ? undefined : Math.max(total - 1, 0)}
          onClose={() => setOpen(null)} onSaved={openNext} />
      )}
    </Page>
  );
}

function StatusIcon({ status }: { status: RecipeStatus }) {
  if (status === 'done') {
    return <svg width="24" height="24" viewBox="0 0 24 24" aria-hidden="true" style={{ flex: 'none' }}><circle cx="12" cy="12" r="10" fill="#15803d" /><path d="M7.5 12.5l3 3 6-6.5" fill="none" stroke="#fff" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" /></svg>;
  }
  if (status === 'review') {
    return <svg width="24" height="24" viewBox="0 0 24 24" aria-hidden="true" style={{ flex: 'none' }}><circle cx="12" cy="12" r="10" fill="#d97706" /><path d="M12 6.5v7M12 16.5v1" stroke="#fff" strokeWidth="2.4" strokeLinecap="round" /></svg>;
  }
  return <svg width="24" height="24" viewBox="0 0 24 24" aria-hidden="true" style={{ flex: 'none' }}><circle cx="12" cy="12" r="9" fill="none" stroke="#8a8a93" strokeWidth="2" strokeDasharray="3 3" /></svg>;
}
