import { useEffect, useMemo, useRef, useState } from 'react';
import { Badge, Box, Button, Grid, HStack, IconButton, Input, Text, VStack } from '@chakra-ui/react';
import { LuMinus, LuPlus, LuTrash2 } from 'react-icons/lu';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerCloseTrigger, DrawerHeader, DrawerBody, DrawerFooter, DrawerTitle,
} from '../components/ui/drawer';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter, DialogTitle,
} from '../components/ui/dialog';
import { toaster } from '../components/ui/toaster';
import { adminApi, type Composition, type CompositionBody, type CompositionKind, type RecipeKind } from '../api/admin';
import { ApiError } from '../api/client';
import { backofficeApi, type Unit } from '../api/backoffice';
import { useHoraDelNegocio } from '../hooks/useHoraDelNegocio';
import { soloFecha } from '../utils/horaDelNegocio';
import { normalize } from '../utils/format';
import { SearchSheet } from './SearchSheet';
import { friendlyQuantity } from './compositionLabel';

// La receta de un producto, un extra o un preparado: lo que sale del almacén cada vez que se vende o
// se prepara. Las palabras de esta hoja son las que decidió el dueño (spec 028, «Vocabulario»).
export function CompositionSheet({ kind, id, name, group, open, onClose, onSaved, pendingLeft }: {
  kind: CompositionKind;
  id: number;
  name: string;
  group?: string;
  open: boolean;
  onClose: () => void;
  // Lo llama la lista de recetas para abrir la siguiente pendiente sin regresar a la lista.
  onSaved?: () => void;
  pendingLeft?: number;
}) {
  const comp = useQuery({
    queryKey: ['admin', 'composition', kind, id],
    queryFn: () => adminApi.composition(kind, id),
    enabled: open,
  });
  const units = useQuery({ queryKey: ['units'], queryFn: () => backofficeApi.units(), enabled: open });
  // Una hoja que nace con `open` puesto no se monta con Chakra 3.37 (AGENTS.md §3, CobrarSheet):
  // Almacén › Insumos la monta y la abre en el mismo toque. Se abre en el render siguiente.
  const [visible, setVisible] = useState(false);
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setVisible(open); }, [open]);
  const [askDiscard, setAskDiscard] = useState(false);
  const [dirty, setDirty] = useState(false);
  const requestClose = () => (dirty ? setAskDiscard(true) : onClose());

  return (
    <>
      <DrawerRoot open={visible} placement="bottom" size="md" onOpenChange={(e) => { if (!e.open) requestClose(); }}>
        <DrawerBackdrop />
        <DrawerContent borderTopRadius="2xl" maxH="94dvh">
          <DrawerCloseTrigger />
          <DrawerHeader pb={1}>
            <HStack align="baseline" gap={3} pr={10}>
              <VStack align="start" gap={0} flex="1" minW={0}>
                <DrawerTitle truncate maxW="100%">
                  {kind === 'ingredient' ? name : `Receta de ${name}`}{group ? ` · ${group}` : ''}
                </DrawerTitle>
              </VStack>
              {!!pendingLeft && <Text fontSize="sm" color="fg.muted" flexShrink={0}>Faltan {pendingLeft} en esta lista</Text>}
            </HStack>
          </DrawerHeader>
          {comp.data && (units.data || units.isError)
            ? <Editor key={comp.dataUpdatedAt} kind={kind} id={id} name={name} data={comp.data}
                onDirty={setDirty} onClose={onClose} onSaved={onSaved} requestClose={requestClose} />
            : <DrawerBody><Text color="fg.muted">{comp.isError ? 'No se pudo abrir la receta.' : 'Cargando…'}</Text></DrawerBody>}
        </DrawerContent>
      </DrawerRoot>
      <DialogRoot open={askDiscard} role="alertdialog" placement="center" onOpenChange={(e) => { if (!e.open) setAskDiscard(false); }}>
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader><DialogTitle>¿Descartar los cambios?</DialogTitle></DialogHeader>
          <DialogBody><Text>Lo que cambiaste en la receta de {name} no se guardará.</Text></DialogBody>
          <DialogFooter>
            <Button variant="outline" minH="44px" onClick={() => setAskDiscard(false)}>Seguir editando</Button>
            <Button colorPalette="red" minH="44px" onClick={() => { setAskDiscard(false); setDirty(false); onClose(); }}>Descartar</Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </>
  );
}

type Mode = 'items' | 'package' | 'product' | 'nothing' | 'bought';

interface Row {
  ingredientId: number;
  name: string;
  text: string; // lo que se tecleó, con coma o punto
  unitId: number;
}

interface Part {
  productId: number;
  quantity: number;
}

const recipeKindOf: Record<CompositionKind, RecipeKind> = { product: 'product', option: 'extra', ingredient: 'prep' };

// La unidad base y la grande de cada tipo: lo que se ofrece en cada renglón.
const UNIT_CHOICES: Record<string, string[]> = { masa: ['g', 'kg'], volumen: ['ml', 'l'], pieza: ['pieza'] };
const unitLabel = (code: string) => (code === 'l' ? 'L' : code === 'pieza' ? 'pza' : code);

// toNumber acepta «0,5» y «0.5»; vacío o inválido es NaN.
function toNumber(text: string): number {
  const t = text.trim().replace(',', '.');
  return t === '' ? NaN : Number(t);
}
function fmt(n: number): string {
  return String(Number(n.toFixed(4)));
}

function Editor({ kind, id, name, data, onDirty, onClose, onSaved, requestClose }: {
  kind: CompositionKind; id: number; name: string; data: Composition;
  onDirty: (d: boolean) => void; onClose: () => void; onSaved?: () => void; requestClose: () => void;
}) {
  const qc = useQueryClient();
  const { zona } = useHoraDelNegocio();
  const isIng = kind === 'ingredient';
  const components = useMemo(() => data.components ?? [], [data.components]);
  // La hoja la monta cuando ya están (CompositionSheet): los renglones nacen en la unidad legible.
  const units = useQuery({ queryKey: ['units'], queryFn: () => backofficeApi.units(), enabled: data.editable });
  const [mode, setMode] = useState<Mode>(() => {
    if (isIng) return data.yield ? 'items' : 'bought';
    if (data.linkedProductId) return 'product';
    if (components.length) return 'package';
    if (data.status === 'confirmed' && !(data.items ?? []).length) return 'nothing';
    return 'items';
  });
  const toRows = (items: Composition['items']): Row[] => (items ?? []).map((it) => ({
    ingredientId: it.ingredientId, name: it.ingredientName, ...friendlyQuantity(Number(it.quantity), it.unitId, units.data?.items ?? []),
  }));
  const [rows, setRows] = useState<Row[]>(() => toRows(data.items));
  // Lo guardado, para marcar contra ello lo nuevo, lo cambiado y lo quitado: al abrir una receta no
  // se distinguía lo que ya llevaba de lo que se acababa de tocar.
  const [original] = useState<Row[]>(() => toRows(data.items));
  const [parts, setParts] = useState<Part[]>(() => components.map((c) => ({ productId: c.productId, quantity: c.quantity })));
  const [linked, setLinked] = useState<number | null>(data.linkedProductId ?? null);
  const [yieldText, setYieldText] = useState(data.yield ? fmt(Number(data.yield)) : '');
  const [dirty, setDirtyState] = useState(false);
  const [removed, setRemoved] = useState<{ undo: () => void; label: string } | null>(null);
  const [applyTwins, setApplyTwins] = useState(true);
  const [picker, setPicker] = useState<null | 'ing' | 'part' | 'link' | 'copy'>(null);
  const [pickerError, setPickerError] = useState('');
  const [failure, setFailure] = useState<null | { conflict: boolean; message: string }>(null);
  // El insumo recién agregado recibe el cursor: lo siguiente que se hace siempre es su cantidad.
  const [justAdded, setJustAdded] = useState<number | null>(null);
  const qtyInputs = useRef(new Map<number, HTMLInputElement>());
  const focusJustAdded = () => { if (justAdded !== null) qtyInputs.current.get(justAdded)?.focus(); };
  // Agregado con un atajo, el foco va directo. Desde el buscador, hasta que termina de cerrarse: antes
  // la hoja de abajo lo recupera y lo deja en su X.
  useEffect(() => {
    if (picker === null) focusJustAdded();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [justAdded]);

  const edit = (fn: () => void) => { fn(); setDirtyState(true); onDirty(true); setFailure(null); };

  const ingredients = useQuery({ queryKey: ['ingredients', 'all'], queryFn: () => backofficeApi.ingredients(), enabled: data.editable });
  const products = useQuery({ queryKey: ['admin', 'products', 'linkable'], queryFn: () => adminApi.products({ status: 'act', limit: 0 }), enabled: data.editable && !isIng });
  const recipeKind = recipeKindOf[kind];
  const sources = useQuery({
    queryKey: ['admin', 'recipes', 'copy-sources', recipeKind],
    queryFn: async () => {
      const [a, b] = await Promise.all([
        adminApi.recipes({ kind: recipeKind, status: 'done', sort: 'az', limit: 100 }),
        adminApi.recipes({ kind: recipeKind, status: 'review', sort: 'az', limit: 100 }),
      ]);
      return [...(a.items ?? []), ...(b.items ?? [])].filter((r) => r.mode === 'items' && r.id !== id);
    },
    enabled: data.editable && mode === 'items' && rows.length === 0,
  });

  const unitById = useMemo(() => new Map((units.data?.items ?? []).map((u) => [u.id, u])), [units.data]);
  const ingById = useMemo(() => new Map((ingredients.data?.items ?? []).map((i) => [i.id, i])), [ingredients.data]);
  const productName = useMemo(() => {
    const m = new Map<number, string>((products.data?.items ?? []).map((p) => [p.id, p.name]));
    for (const c of components) if (!m.has(c.productId)) m.set(c.productId, c.productName);
    if (data.linkedProductId && data.linkedProductName) m.set(data.linkedProductId, data.linkedProductName);
    return m;
  }, [products.data, components, data.linkedProductId, data.linkedProductName]);

  const origById = useMemo(() => new Map(original.map((o) => [o.ingredientId, o])), [original]);
  // En la unidad base: 200 ml y 0.2 L son lo mismo y no cuentan como cambio.
  const baseQty = (r: Row) => toNumber(r.text) * Number(unitById.get(r.unitId)?.toBase ?? 1);
  const changeOf = (r: Row): 'new' | 'changed' | '' => {
    const o = origById.get(r.ingredientId);
    if (!o) return 'new';
    return Math.abs(baseQty(o) - baseQty(r)) <= 1e-6 * Math.max(1, Math.abs(baseQty(o))) ? '' : 'changed';
  };
  const dropped = mode === 'items' ? original.filter((o) => !rows.some((x) => x.ingredientId === o.ingredientId)) : [];
  const restore = (o: Row) => edit(() => setRows((rs) => {
    const at = original.indexOf(o);
    return [...rs.slice(0, at), o, ...rs.slice(at)];
  }));
  const kindOfRow = (r: Row) => ingById.get(r.ingredientId)?.baseUnitKind ?? unitById.get(r.unitId)?.kind ?? 'pieza';
  const unitChoices = (r: Row): Unit[] => {
    const k = kindOfRow(r);
    const codes = UNIT_CHOICES[k] ?? [];
    return (units.data?.items ?? []).filter((u) => u.kind === k && (codes.includes(u.code) || u.id === r.unitId))
      .sort((a, b) => Number(a.toBase) - Number(b.toBase));
  };

  const addIngredient = (ingId: number): boolean => {
    if (rows.some((r) => r.ingredientId === ingId)) {
      setPickerError(`${ingById.get(ingId)?.name ?? 'Ese insumo'} ya está en la receta.`);
      return false;
    }
    const ing = ingById.get(ingId);
    if (!ing) return false;
    edit(() => setRows((rs) => [...rs, { ingredientId: ing.id, name: ing.name, text: ing.baseUnitKind === 'pieza' ? '1' : '', unitId: ing.baseUnitId }]));
    setJustAdded(ing.id);
    return true;
  };
  const createIngredient = useMutation({
    mutationFn: (b: { name: string; baseUnitId: number }) => backofficeApi.createIngredient(b),
    onSuccess: (ing) => {
      qc.setQueryData(['ingredients', 'all'], (old: { items: typeof ing[] } | undefined) => ({ items: [...(old?.items ?? []), ing] }));
      edit(() => setRows((rs) => [...rs, { ingredientId: ing.id, name: ing.name, text: ing.baseUnitKind === 'pieza' ? '1' : '', unitId: ing.baseUnitId }]));
      setJustAdded(ing.id);
      setPicker(null);
      toaster.create({ title: `Insumo creado: ${ing.name}`, type: 'success' });
    },
    onError: (e) => setPickerError(String(e instanceof Error ? e.message : e)),
  });

  const copyFrom = async (srcId: number) => {
    const src = await adminApi.composition(kind, srcId);
    edit(() => {
      setMode('items');
      setRows(toRows(src.items));
    });
    setPicker(null);
  };

  const rowValid = (r: Row) => toNumber(r.text) > 0;
  const yieldNum = toNumber(yieldText);
  const valid = mode === 'items'
    ? rows.length > 0 && rows.every(rowValid) && (!isIng || yieldNum > 0)
    : mode === 'package' ? parts.length > 0
      : mode === 'product' ? linked !== null
        : true;
  const confirmOnly = data.status === 'estimated' && !dirty;
  const twins = kind === 'option' ? (data.sameName ?? []) : [];
  const offerTwins = twins.length > 0 && (mode === 'items' || mode === 'product' || mode === 'nothing') && !confirmOnly;

  const body = (): CompositionBody => ({
    items: mode === 'items' ? rows.map((r) => ({ ingredientId: r.ingredientId, quantity: fmt(toNumber(r.text)), unitId: r.unitId })) : [],
    linkedProductId: mode === 'product' ? linked : null,
    components: mode === 'package' ? parts : [],
    ...(isIng ? { yield: mode === 'items' ? fmt(yieldNum) : null } : {}),
    ...(offerTwins && applyTwins
      ? { alsoOptionIds: twins.map((t) => t.id), alsoBasedOn: Object.fromEntries(twins.map((t) => [t.id, t.stamp ?? ''])) }
      : {}),
    basedOn: data.stamp ?? '',
  });

  const save = useMutation({
    mutationFn: async () => {
      if (confirmOnly) return adminApi.confirmComposition(kind, id);
      if (mode === 'nothing') {
        // «No gasta insumos» es una receta vacía confirmada por una persona.
        await adminApi.saveComposition(kind, id, { ...body(), items: [], components: [], linkedProductId: null });
        if (offerTwins && applyTwins) for (const t of twins) await adminApi.confirmComposition(kind, t.id);
        return adminApi.confirmComposition(kind, id);
      }
      return adminApi.saveComposition(kind, id, body());
    },
    onSuccess: (c) => {
      qc.setQueryData(['admin', 'composition', kind, id], c);
      qc.invalidateQueries({ queryKey: ['admin'] });
      qc.invalidateQueries({ queryKey: ['ingredients'] });
      const extra = offerTwins && applyTwins ? ` (y ${twins.length} con el mismo nombre)` : '';
      toaster.create({ title: `${confirmOnly ? 'Confirmada' : 'Lista'}: ${name}${extra}`, type: 'success' });
      onDirty(false);
      if (onSaved) onSaved(); else onClose();
    },
    onError: (e) => {
      if (e instanceof ApiError) {
        setFailure({ conflict: e.status === 409, message: e.message });
      } else {
        setFailure({ conflict: false, message: 'No se guardó: no hay conexión. Lo que escribiste sigue aquí.' });
      }
    },
  });

  if (!data.editable) {
    return (
      <>
        <DrawerBody>
          <Box p={4} borderRadius="xl" bg="blue.50" borderWidth="1px" borderColor="blue.200">
            <Text>
              {data.reason === 'package_choices'
                ? 'Este combo deja elegir entre productos o tiene un hueco sin producto. Cada venta descuenta lo que lleva por omisión.'
                : `Se descuenta solo: cada venta baja 1 pieza de ${name}. No necesita receta.`}
            </Text>
          </Box>
        </DrawerBody>
        <DrawerFooter><Button minH="48px" onClick={onClose}>Entendido</Button></DrawerFooter>
      </>
    );
  }

  const modes: { k: Mode; title: string; example: string }[] = isIng
    ? [{ k: 'items', title: 'Es un preparado', example: 'Se hace aquí con otros insumos' }, { k: 'bought', title: 'Se compra hecho', example: 'Baja de sus existencias' }]
    : kind === 'option'
      ? [{ k: 'items', title: 'Lleva insumos', example: 'Ej. 45 g de tapioca' }, { k: 'product', title: 'Es otro producto', example: 'Ej. la Coca del combo' }, { k: 'nothing', title: 'No gasta insumos', example: 'Ej. «Sin hielo»' }]
      : [{ k: 'items', title: 'Lleva insumos', example: 'Ej. leche, café, vaso' }, { k: 'package', title: 'Es un combo', example: 'Productos con sus piezas' }, { k: 'nothing', title: 'No gasta insumos', example: 'Nada sale del almacén' }];

  const mostUsed = (ingredients.data?.items ?? [])
    .filter((i) => (i.recipeUses ?? 0) > 0 && !rows.some((r) => r.ingredientId === i.id) && !(isIng && i.id === id))
    .sort((a, b) => (b.recipeUses ?? 0) - (a.recipeUses ?? 0)).slice(0, 3);
  const firstWord = normalize(name.split(' ')[0] ?? '');
  const copyChips = (sources.data ?? []).slice().sort((a, b) => Number(normalize(b.name).startsWith(firstWord)) - Number(normalize(a.name).startsWith(firstWord))).slice(0, 2);

  const preview = mode === 'nothing' ? 'nada.'
    : mode === 'bought' ? 'baja de sus existencias.'
      : mode === 'product' ? (linked ? `1 ${productName.get(linked) ?? ''}` : '— todavía nada —')
        : mode === 'package' ? (parts.length ? parts.map((p) => `${p.quantity} ${productName.get(p.productId) ?? ''}`).join(' + ') : '— todavía nada —')
          : rows.filter(rowValid).map((r) => `${r.text.replace(',', '.')} ${unitLabel(unitById.get(r.unitId)?.code ?? '')} ${r.name}`).join(' · ') || '— todavía nada —';
  // Sin tocar nada no hay error que señalar: el aviso en naranja antes de empezar se leía como falta.
  const hint = confirmOnly || valid || !dirty ? ''
    : mode === 'items' ? (rows.length === 0 ? 'Agrega al menos un insumo.' : !rows.every(rowValid) ? 'Escribe la cantidad de cada insumo.' : 'Falta cuánto rinde.')
      : mode === 'package' ? 'Agrega al menos un producto.' : 'Elige el producto.';
  const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`;
  const pending: string[] = mode === 'items'
    ? ([[rows.filter((x) => changeOf(x) === 'new').length, 'nuevo'],
      [rows.filter((x) => changeOf(x) === 'changed').length, 'cambiado'],
      [dropped.length, 'quitado']] as [number, string][]).filter(([n]) => n > 0).map(([n, w]) => plural(n, w))
    : [];
  const unsaved = pending.length ? `Sin guardar: ${pending.join(' · ')}` : '';
  const remove = (label: string, undo: () => void, drop: () => void) => edit(() => { setRemoved({ label, undo }); drop(); });

  return (
    <>
      <DrawerBody pt={0}>
        <VStack align="stretch" gap={3}>
          <Text fontSize="sm" color="fg.muted">
            {isIng ? (mode === 'items' ? 'Para prepararlo se usa:' : 'Insumo') : 'Al vender 1, del almacén sale:'}
          </Text>
          {data.status === 'estimated' && (
            <HStack px={3} py={2} borderRadius="lg" bg="orange.50" borderWidth="1px" borderColor="orange.200" fontSize="sm">
              <Text>Esta receta se cargó de tu sistema anterior. Revísala: si está bien, toca <b>Está bien</b>.</Text>
            </HStack>
          )}
          {data.status === 'confirmed' && data.confirmedBy && (
            <Text fontSize="sm" color="fg.muted">
              La guardó {data.confirmedBy}{data.confirmedAt ? ` el ${soloFecha(data.confirmedAt, zona)}` : ''}.
            </Text>
          )}
          {failure && (
            <HStack role="alert" px={3} py={2} borderRadius="lg" bg="red.50" borderWidth="1px" borderColor="red.200" color="red.800" fontSize="sm">
              <Text flex="1">{failure.message}</Text>
              {failure.conflict && (
                <Button size="sm" minH="44px" variant="outline" onClick={() => { onDirty(false); qc.invalidateQueries({ queryKey: ['admin', 'composition', kind, id] }); }}>
                  Abrir de nuevo
                </Button>
              )}
            </HStack>
          )}

          <Grid templateColumns={`repeat(${modes.length}, minmax(0, 1fr))`} gap={2}>
            {modes.map((m) => (
              <Button key={m.k} h="auto" minH="64px" py={2} px={3} variant="outline" justifyContent="start" textAlign="left"
                data-active={mode === m.k ? '' : undefined} aria-pressed={mode === m.k}
                borderWidth={mode === m.k ? '2px' : '1px'} borderColor={mode === m.k ? 'red.600' : 'border'}
                bg={mode === m.k ? 'red.50' : undefined}
                onClick={() => edit(() => { setMode(m.k); setRemoved(null); })}>
                <VStack align="start" gap={0}>
                  <Text fontWeight="600">{m.title}</Text>
                  <Text fontSize="xs" color="fg.muted" fontWeight="normal" whiteSpace="normal">{m.example}</Text>
                </VStack>
              </Button>
            ))}
          </Grid>

          {mode === 'items' && (
            <VStack align="stretch" gap={2}>
              {rows.length === 0 && (copyChips.length > 0 || (sources.data ?? []).length > 0) && (
                <HStack wrap="wrap" gap={2}>
                  <Text fontSize="sm" color="fg.muted">Copiar la receta de</Text>
                  {copyChips.map((c) => (
                    <Button key={c.id} size="sm" minH="44px" variant="subtle" colorPalette="blue" borderRadius="full" onClick={() => copyFrom(c.id)}>{c.name}</Button>
                  ))}
                  <Button size="sm" minH="44px" variant="outline" colorPalette="blue" borderRadius="full" onClick={() => setPicker('copy')}>Buscar otra…</Button>
                </HStack>
              )}
              {(rows.length > 0 || dropped.length > 0) && (
                <HStack px={3} fontSize="xs" color="fg.muted" textTransform="uppercase" letterSpacing="wide">
                  <Text flex="1">{original.length ? `Guardado · ${plural(original.length, 'insumo')}` : 'Insumo'}</Text>
                  <Text>{isIng ? 'Para prepararlo' : 'Por cada venta'}</Text>
                  <Box w="148px" />
                </HStack>
              )}
              {rows.map((r, i) => {
                // El recién agregado todavía no tiene cantidad porque es lo que sigue, no un error.
                const bad = dirty && !rowValid(r) && !(r.ingredientId === justAdded && r.text === '');
                const change = changeOf(r);
                const before = origById.get(r.ingredientId);
                return (
                  <HStack key={`${r.ingredientId}-${i}`} data-row gap={2} pl={3} pr={1} py={1} borderWidth="1px" borderRadius="lg"
                    borderLeftWidth={change ? '4px' : '1px'}
                    borderColor={bad ? 'red.300' : 'border.muted'} borderLeftColor={change === 'new' ? 'green.500' : change === 'changed' ? 'orange.400' : undefined}
                    bg={bad ? 'red.50' : undefined}>
                    <VStack flex="1" minW={0} align="start" gap={0}>
                      <HStack gap={2} maxW="100%">
                        <Text truncate>{r.name}</Text>
                        {change === 'new' && <Badge colorPalette="green" variant="subtle" flexShrink={0}>Nuevo</Badge>}
                      </HStack>
                      {change === 'changed' && before && (
                        <Text fontSize="xs" color="orange.700">Antes {before.text} {unitLabel(unitById.get(before.unitId)?.code ?? '')}</Text>
                      )}
                    </VStack>
                    <Input aria-label={`Cantidad de ${r.name}`} inputMode="decimal" w="96px" minH="44px" textAlign="end" fontWeight="600"
                      borderColor={bad ? 'red.500' : undefined} value={r.text}
                      ref={(el) => { if (el) qtyInputs.current.set(r.ingredientId, el); else qtyInputs.current.delete(r.ingredientId); }}
                      onFocus={(e) => e.currentTarget.scrollIntoView?.({ block: 'center' })}
                      onChange={(e) => { const v = e.target.value.replace(/[^0-9.,]/g, ''); edit(() => setRows((rs) => rs.map((x, j) => (j === i ? { ...x, text: v } : x)))); }} />
                    <HStack gap={0} borderWidth="1px" borderRadius="md" overflow="hidden">
                      {unitChoices(r).map((u) => (
                        <Button key={u.id} size="sm" minH="44px" minW="46px" borderRadius={0} variant={u.id === r.unitId ? 'solid' : 'ghost'}
                          aria-pressed={u.id === r.unitId}
                          onClick={() => edit(() => setRows((rs) => rs.map((x, j) => {
                            if (j !== i || x.unitId === u.id) return x;
                            // Cambiar de unidad convierte el número: 200 ml son 0.2 L, no 200 L.
                            const from = unitById.get(x.unitId); const n = toNumber(x.text);
                            const text = from && n > 0 ? fmt((n * Number(from.toBase)) / Number(u.toBase)) : x.text;
                            return { ...x, unitId: u.id, text };
                          })))}>
                          {unitLabel(u.code)}
                        </Button>
                      ))}
                    </HStack>
                    <IconButton aria-label={`Quitar ${r.name}`} variant="ghost" colorPalette="red" minH="44px" minW="44px" ml={3}
                      onClick={() => (before
                        // Lo guardado se queda a la vista, tachado, hasta guardar: ahí se regresa.
                        ? edit(() => setRows((rs) => rs.filter((_, j) => j !== i)))
                        : remove(r.name, () => setRows((rs) => [...rs.slice(0, i), r, ...rs.slice(i)]), () => setRows((rs) => rs.filter((_, j) => j !== i))))}>
                      <LuTrash2 />
                    </IconButton>
                  </HStack>
                );
              })}
              {dropped.map((o) => (
                <HStack key={`quitado-${o.ingredientId}`} data-row gap={2} pl={3} pr={1} py={1} borderWidth="1px" borderLeftWidth="4px"
                  borderRadius="lg" borderStyle="dashed" borderColor="border.muted" borderLeftColor="red.400" bg="bg.subtle" color="fg.muted">
                  <VStack flex="1" minW={0} align="start" gap={0}>
                    <Text truncate textDecoration="line-through">{o.name}</Text>
                    <Text fontSize="xs">Antes {o.text} {unitLabel(unitById.get(o.unitId)?.code ?? '')} · se quita al guardar</Text>
                  </VStack>
                  <Button aria-label={`Regresar ${o.name}`} variant="outline" minH="44px" onClick={() => restore(o)}>Regresar</Button>
                </HStack>
              ))}
              {isIng && (
                <HStack gap={2} pl={3} pr={1} py={1} borderRadius="lg" bg="blue.50" borderWidth="1px" borderColor="blue.200">
                  <Text flex="1">Con esto salen</Text>
                  <Input aria-label="Cuánto rinde" inputMode="decimal" w="96px" minH="44px" textAlign="end" fontWeight="600" bg="white"
                    borderColor={dirty && !(yieldNum > 0) ? 'red.500' : undefined} value={yieldText}
                    onFocus={(e) => e.currentTarget.scrollIntoView?.({ block: 'center' })}
                    onChange={(e) => { const v = e.target.value.replace(/[^0-9.,]/g, ''); edit(() => setYieldText(v)); }} />
                  <Text w="58px">{unitLabel(data.yieldUnitCode ?? '')}</Text>
                </HStack>
              )}
              <HStack wrap="wrap" gap={2}>
                <Button minH="44px" variant="outline" onClick={() => { setPickerError(''); setPicker('ing'); }}><LuPlus /> Agregar insumo</Button>
                {mostUsed.length > 0 && <Text fontSize="sm" color="fg.muted">Más usados:</Text>}
                {mostUsed.map((i) => (
                  <Button key={i.id} size="sm" minH="44px" variant="outline" borderStyle="dashed" borderRadius="full" onClick={() => addIngredient(i.id)}>+ {i.name}</Button>
                ))}
              </HStack>
            </VStack>
          )}

          {mode === 'package' && (
            <VStack align="stretch" gap={2}>
              {parts.map((p, i) => {
                const pname = productName.get(p.productId) ?? '';
                return (
                  <HStack key={`${p.productId}-${i}`} gap={2} pl={3} pr={1} py={1} borderWidth="1px" borderRadius="lg" borderColor="border.muted">
                    <Text flex="1" minW={0} truncate>{pname}</Text>
                    {/* Piezas con − y +: en un combo casi siempre son 1 o 2, y teclear abre el teclado. */}
                    <IconButton aria-label={`Una pieza menos de ${pname}`} variant="outline" minH="44px" minW="44px" disabled={p.quantity <= 1}
                      onClick={() => edit(() => setParts((ps) => ps.map((x, j) => (j === i ? { ...x, quantity: x.quantity - 1 } : x))))}><LuMinus /></IconButton>
                    <Text w="72px" textAlign="center" fontWeight="600">{p.quantity} {p.quantity === 1 ? 'pieza' : 'piezas'}</Text>
                    <IconButton aria-label={`Una pieza más de ${pname}`} variant="outline" minH="44px" minW="44px" disabled={p.quantity >= 99}
                      onClick={() => edit(() => setParts((ps) => ps.map((x, j) => (j === i ? { ...x, quantity: x.quantity + 1 } : x))))}><LuPlus /></IconButton>
                    <IconButton aria-label={`Quitar ${pname}`} variant="ghost" colorPalette="red" minH="44px" minW="44px" ml={3}
                      onClick={() => remove(pname, () => setParts((ps) => [...ps.slice(0, i), p, ...ps.slice(i)]), () => setParts((ps) => ps.filter((_, j) => j !== i)))}>
                      <LuTrash2 />
                    </IconButton>
                  </HStack>
                );
              })}
              <Box><Button minH="44px" variant="outline" onClick={() => setPicker('part')}><LuPlus /> Agregar producto al combo</Button></Box>
            </VStack>
          )}

          {mode === 'product' && (
            <HStack gap={3}>
              <Text color="fg.muted">Es:</Text>
              <Button minH="48px" variant="outline" borderWidth={linked ? '2px' : '1px'} borderStyle={linked ? 'solid' : 'dashed'}
                onClick={() => setPicker('link')}>{linked ? productName.get(linked) : 'Elegir producto'} ▾</Button>
            </HStack>
          )}
          {mode === 'nothing' && <Box p={3} borderRadius="lg" bg="bg.muted" color="fg.muted">Al venderlo no sale nada del almacén.</Box>}
          {mode === 'bought' && <Box p={3} borderRadius="lg" bg="bg.muted" color="fg.muted">Se compra hecho: cada vez que una receta lo usa, baja de sus existencias.</Box>}

          {removed && (
            <HStack justify="space-between" bg="bg.muted" borderRadius="md" px={3}>
              <Text fontSize="sm">Quitaste {removed.label}</Text>
              <Button variant="ghost" minH="44px" onClick={() => { removed.undo(); setRemoved(null); }}>Deshacer</Button>
            </HStack>
          )}
        </VStack>
      </DrawerBody>

      {/* Siempre a la vista: esconderla al teclear movía la hoja, y el botón que se iba a tocar
          quedaba en otro lugar bajo el dedo. */}
      <HStack mx={6} px={3} py={2} borderRadius="lg" bg="green.50" borderWidth="1px" borderColor="green.200" fontSize="sm" align="baseline">
        <Text fontWeight="700" flexShrink={0}>{isIng && mode === 'items' ? `Para ${yieldText || '…'} ${unitLabel(data.yieldUnitCode ?? '')}:` : mode === 'bought' ? 'Cada uso:' : 'Al vender 1:'}</Text>
        <Text truncate>{preview}</Text>
      </HStack>
      <DrawerFooter gap={2}>
        {offerTwins && (
          <Button minH="44px" variant="outline" aria-pressed={applyTwins} onClick={() => setApplyTwins((v) => !v)}>
            {applyTwins ? '☑' : '☐'} También en «{twins.map((t) => t.group).join('», «')}»
          </Button>
        )}
        <Text flex="1" fontSize="sm" color={hint ? 'orange.700' : 'fg.muted'}>{hint || unsaved}</Text>
        <Button variant="ghost" minH="48px" onClick={requestClose}>Cancelar</Button>
        <Button minH="48px" colorPalette="red" disabled={!confirmOnly && !valid} loading={save.isPending} onClick={() => save.mutate()}>
          {failure && !failure.conflict ? 'Reintentar' : confirmOnly ? 'Está bien' : 'Guardar'}
        </Button>
      </DrawerFooter>

      <SearchSheet open={picker === 'ing'} title="Agregar insumo" error={pickerError} restoreFocus={false} onExitComplete={focusJustAdded}
        options={(ingredients.data?.items ?? []).filter((i) => !(isIng && i.id === id))
          .sort((a, b) => (b.recipeUses ?? 0) - (a.recipeUses ?? 0) || a.name.localeCompare(b.name, 'es'))
          .map((i) => ({ value: String(i.id), label: i.name, hint: `${i.isPrep ? 'preparado · ' : ''}${unitLabel(i.baseUnitCode)}` }))}
        onPick={(v) => { if (addIngredient(Number(v))) setPicker(null); }}
        onClose={() => setPicker(null)}
        renderCreate={(q) => (
          <VStack align="stretch" gap={2} py={2}>
            <Text>Crear el insumo <b>«{q}»</b>. ¿En qué se mide?</Text>
            <HStack gap={2}>
              {(['g', 'ml', 'pieza'] as const).map((code) => {
                const u = (units.data?.items ?? []).find((x) => x.code === code);
                return (
                  <Button key={code} minH="44px" variant="outline" disabled={!u} loading={createIngredient.isPending}
                    onClick={() => u && createIngredient.mutate({ name: q, baseUnitId: u.id })}>
                    {code === 'g' ? 'Gramos (g)' : code === 'ml' ? 'Mililitros (ml)' : 'Piezas'}
                  </Button>
                );
              })}
            </HStack>
          </VStack>
        )} />
      <SearchSheet open={picker === 'part' || picker === 'link'} title={picker === 'part' ? 'Agregar producto al combo' : '¿Cuál producto es?'}
        options={(products.data?.items ?? [])
          // Un combo no lleva combos ni a sí mismo; un extra sí puede ser un combo.
          .filter((p) => picker === 'link' || (p.type !== 'combo' && p.id !== id && !parts.some((x) => x.productId === p.id)))
          .map((p) => ({ value: String(p.id), label: p.name, hint: p.type === 'combo' ? 'combo' : '' }))}
        onPick={(v) => {
          const pid = Number(v);
          if (picker === 'part') edit(() => setParts((ps) => [...ps, { productId: pid, quantity: 1 }]));
          else edit(() => setLinked(pid));
          setPicker(null);
        }}
        onClose={() => setPicker(null)} />
      <SearchSheet open={picker === 'copy'} title="Copiar la receta de…" emptyText="No hay otra receta con insumos para copiar."
        options={(sources.data ?? []).map((r) => ({ value: String(r.id), label: r.name, hint: `${r.lines} insumos` }))}
        onPick={(v) => copyFrom(Number(v))} onClose={() => setPicker(null)} />
    </>
  );
}

