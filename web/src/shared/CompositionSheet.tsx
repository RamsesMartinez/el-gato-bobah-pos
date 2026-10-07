import { useMemo, useState } from 'react';
import { Badge, Box, Button, HStack, IconButton, Input, Text, VStack } from '@chakra-ui/react';
import { LuMinus, LuPlus, LuTrash2 } from 'react-icons/lu';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerCloseTrigger, DrawerHeader, DrawerBody, DrawerFooter, DrawerTitle,
} from '../components/ui/drawer';
import { Picker } from '../components/Picker';
import { toaster } from '../components/ui/toaster';
import { adminApi, type Composition, type CompositionKind } from '../api/admin';
import { backofficeApi } from '../api/backoffice';
import { useHoraDelNegocio } from '../hooks/useHoraDelNegocio';
import { soloFecha } from '../utils/horaDelNegocio';

// «Qué lleva»: lo que un producto o un extra descuenta del almacén cada vez que se vende. Es una
// hoja propia y no una sección del diálogo, para no meter un diálogo dentro de otro.
export function CompositionSheet({ kind, id, name, open, onClose }: {
  kind: CompositionKind;
  id: number;
  name: string;
  open: boolean;
  onClose: () => void;
}) {
  const comp = useQuery({
    queryKey: ['admin', 'composition', kind, id],
    queryFn: () => adminApi.composition(kind, id),
    enabled: open,
  });
  return (
    <DrawerRoot open={open} placement="bottom" size="md" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl" maxH="90dvh">
        <DrawerCloseTrigger />
        <DrawerHeader pb={2}>
          <DrawerTitle>Qué lleva · {name}</DrawerTitle>
        </DrawerHeader>
        {comp.data
          ? <Editor key={comp.dataUpdatedAt} kind={kind} id={id} data={comp.data} onClose={onClose} />
          : <DrawerBody><Text color="fg.muted">{comp.isError ? 'No se pudo cargar.' : 'Cargando…'}</Text></DrawerBody>}
      </DrawerContent>
    </DrawerRoot>
  );
}

interface Row {
  ingredientId: number | null;
  quantity: string;
  unitId: number | null;
}

interface Part {
  productId: number | null;
  quantity: number;
}

type Mode = 'items' | 'product' | 'package';

// Lo que hace cada modo, en una línea, para quien captura.
const AYUDA: Record<Mode, string> = {
  items: 'Cada venta descuenta del almacén estos insumos.',
  product: 'Cada venta descuenta lo que lleva ese producto, como si se vendiera solo.',
  package: 'Cada venta descuenta lo que lleva cada producto, por las piezas indicadas.',
};

type Opt = { value: string; label: string };

function Editor({ kind, id, data, onClose }: { kind: CompositionKind; id: number; data: Composition; onClose: () => void }) {
  const qc = useQueryClient();
  const components = useMemo(() => data.components ?? [], [data.components]);
  const [mode, setMode] = useState<Mode>(
    data.linkedProductId ? 'product' : components.length > 0 ? 'package' : 'items',
  );
  const [rows, setRows] = useState<Row[]>(() => (data.items ?? []).map((it) => ({
    ingredientId: it.ingredientId, quantity: it.quantity, unitId: it.unitId,
  })));
  const [parts, setParts] = useState<Part[]>(() => components.map((c) => ({ productId: c.productId, quantity: c.quantity })));
  const [linked, setLinked] = useState<number | null>(data.linkedProductId ?? null);
  const [dirty, setDirty] = useState(false);
  // Lo último que se quitó, para deshacerlo con un toque: quitar es un toque y recapturar son varios.
  const [removed, setRemoved] = useState<{ undo: () => void; name: string } | null>(null);

  const ingredients = useQuery({ queryKey: ['ingredients', 'all'], queryFn: () => backofficeApi.ingredients(), enabled: data.editable });
  const units = useQuery({ queryKey: ['units'], queryFn: () => backofficeApi.units(), enabled: data.editable });
  const products = useQuery({
    queryKey: ['admin', 'products', 'linkable'],
    queryFn: () => adminApi.products({ status: 'act', limit: 0 }),
    enabled: data.editable,
  });

  const ingById = useMemo(() => new Map((ingredients.data?.items ?? []).map((i) => [i.id, i])), [ingredients.data]);
  // Los que ya trae la composición, aunque estén inactivos: si no, el renglón se vería vacío.
  const ingOptions = useMemo(() => {
    const opts: Opt[] = (ingredients.data?.items ?? []).map((i) => ({ value: String(i.id), label: i.name }));
    for (const it of data.items ?? []) {
      if (!ingById.has(it.ingredientId)) opts.push({ value: String(it.ingredientId), label: it.ingredientName });
    }
    return opts;
  }, [ingredients.data, ingById, data.items]);
  const productName = useMemo(() => {
    const m = new Map((products.data?.items ?? []).map((p) => [p.id, p.name]));
    for (const c of components) if (!m.has(c.productId)) m.set(c.productId, c.productName);
    if (data.linkedProductId && data.linkedProductName) m.set(data.linkedProductId, data.linkedProductName);
    return m;
  }, [products.data, components, data.linkedProductId, data.linkedProductName]);
  // Un extra se liga a cualquier producto, paquete incluido; un paquete solo lleva productos sueltos
  // y nunca a sí mismo.
  const linkOptions: Opt[] = (products.data?.items ?? []).map((p) => ({ value: String(p.id), label: p.name }));
  const partOptions: Opt[] = (products.data?.items ?? [])
    .filter((p) => p.type !== 'combo' && p.id !== id)
    .map((p) => ({ value: String(p.id), label: p.name }));
  const withCurrent = (opts: Opt[], current: number | null): Opt[] =>
    current && !opts.some((o) => o.value === String(current))
      ? [...opts, { value: String(current), label: productName.get(current) ?? '' }]
      : opts;

  const edit = (fn: () => void) => { fn(); setDirty(true); };
  const done = (c: Composition, title: string) => {
    qc.setQueryData(['admin', 'composition', kind, id], c);
    qc.invalidateQueries({ queryKey: ['admin'] });
    toaster.create({ title, type: 'success' });
    onClose();
  };
  const fail = (e: unknown) => toaster.create({ title: 'No se guardó', description: String(e), type: 'error' });

  const save = useMutation({
    mutationFn: () => adminApi.saveComposition(kind, id, {
      items: mode === 'items'
        ? rows.map((r) => ({ ingredientId: r.ingredientId!, quantity: r.quantity, unitId: r.unitId! }))
        : [],
      linkedProductId: mode === 'product' ? linked : null,
      components: mode === 'package' ? parts.map((p) => ({ productId: p.productId!, quantity: p.quantity })) : [],
    }),
    onSuccess: (c) => done(c, 'Guardado'),
    onError: fail,
  });
  const confirm = useMutation({
    mutationFn: () => adminApi.confirmComposition(kind, id),
    onSuccess: (c) => done(c, 'Confirmado'),
    onError: fail,
  });

  if (!data.editable) {
    return (
      <>
        <DrawerBody>
          <Text>Este producto descuenta sus propias existencias: cada venta baja una pieza de él.</Text>
        </DrawerBody>
        <DrawerFooter><Button minH="44px" onClick={onClose}>Cerrar</Button></DrawerFooter>
      </>
    );
  }

  const valid = mode === 'product'
    ? linked !== null
    : mode === 'package'
      ? parts.every((p) => p.productId !== null)
      : rows.every((r) => r.ingredientId !== null && r.unitId !== null && Number(r.quantity) > 0);
  const empty = mode === 'items' && rows.length === 0;
  const modes: { k: Mode; label: string }[] = kind === 'option'
    ? [{ k: 'items', label: 'Lleva insumos' }, { k: 'product', label: 'Es un producto' }]
    : [{ k: 'items', label: 'Lleva insumos' }, { k: 'package', label: 'Es un paquete' }];

  const remove = (label: string, undo: () => void, drop: () => void) => edit(() => {
    setRemoved({ name: label, undo });
    drop();
  });
  // Separado de lo último que se toca en cada renglón, y con «Deshacer» abajo.
  const removeButton = (label: string, onClick: () => void) => (
    <IconButton aria-label={`Quitar ${label}`} variant="ghost" colorPalette="red" minH="44px" minW="44px" ml={3} onClick={onClick}>
      <LuTrash2 />
    </IconButton>
  );

  return (
    <>
      <DrawerBody>
        <VStack align="stretch" gap={3}>
          <StatusLine data={data} />
          <HStack gap={2}>
            {modes.map((m) => (
              <Button key={m.k} flex="1" minH="44px" variant={mode === m.k ? 'solid' : 'outline'}
                data-active={mode === m.k ? '' : undefined}
                onClick={() => edit(() => { setMode(m.k); setRemoved(null); })}>{m.label}</Button>
            ))}
          </HStack>
          <Text fontSize="sm" color="fg.muted">{AYUDA[mode]}</Text>

          {mode === 'product' && (
            <Picker title="Producto" placeholder="Elegir producto"
              value={linked ? String(linked) : ''} options={withCurrent(linkOptions, linked)}
              onChange={(v) => edit(() => setLinked(Number(v)))} />
          )}

          {mode === 'items' && rows.map((r, i) => {
            const ing = r.ingredientId ? ingById.get(r.ingredientId) : undefined;
            const ingName = ing?.name ?? (data.items ?? []).find((x) => x.ingredientId === r.ingredientId)?.ingredientName ?? '';
            const unitOpts = (units.data?.items ?? [])
              .filter((u) => !ing || u.kind === ing.baseUnitKind)
              .map((u) => ({ value: String(u.id), label: u.code }));
            const set = (patch: Partial<Row>) => edit(() => setRows((rs) => rs.map((x, j) => (j === i ? { ...x, ...patch } : x))));
            return (
              <HStack key={i} gap={2} align="center">
                <Box flex="1" minW={0}>
                  <Picker title="Insumo" placeholder="Elegir insumo" value={r.ingredientId ? String(r.ingredientId) : ''}
                    options={ingOptions}
                    onChange={(v) => {
                      const picked = ingById.get(Number(v));
                      set({ ingredientId: Number(v), unitId: picked?.baseUnitId ?? r.unitId });
                    }} />
                </Box>
                <Input aria-label="Cantidad" type="number" inputMode="decimal" min={0} w="96px" minH="44px"
                  value={r.quantity} onChange={(e) => set({ quantity: e.target.value })} />
                <Box w="88px">
                  <Picker title="Unidad" placeholder="Unidad" value={r.unitId ? String(r.unitId) : ''}
                    options={unitOpts} onChange={(v) => set({ unitId: Number(v) })} />
                </Box>
                {removeButton(ingName, () => remove(ingName || 'un renglón',
                  () => setRows((rs) => [...rs.slice(0, i), r, ...rs.slice(i)]),
                  () => setRows((rs) => rs.filter((_, j) => j !== i))))}
              </HStack>
            );
          })}

          {mode === 'package' && parts.map((p, i) => {
            const name = p.productId ? productName.get(p.productId) ?? '' : '';
            const set = (patch: Partial<Part>) => edit(() => setParts((ps) => ps.map((x, j) => (j === i ? { ...x, ...patch } : x))));
            return (
              <HStack key={i} gap={2} align="center">
                <Box flex="1" minW={0}>
                  <Picker title="Producto" placeholder="Elegir producto" value={p.productId ? String(p.productId) : ''}
                    options={withCurrent(partOptions, p.productId)} onChange={(v) => set({ productId: Number(v) })} />
                </Box>
                {/* Piezas con − y +: en un paquete casi siempre son 1 o 2, y teclear abre el teclado
                    y tapa media tableta. */}
                <IconButton aria-label={`Una pieza menos de ${name}`} variant="outline" minH="44px" minW="44px"
                  disabled={p.quantity <= 1} onClick={() => set({ quantity: p.quantity - 1 })}><LuMinus /></IconButton>
                <Text w="32px" textAlign="center" fontWeight="600">{p.quantity}</Text>
                <IconButton aria-label={`Una pieza más de ${name}`} variant="outline" minH="44px" minW="44px"
                  disabled={p.quantity >= 99} onClick={() => set({ quantity: p.quantity + 1 })}><LuPlus /></IconButton>
                {removeButton(name, () => remove(name || 'un producto',
                  () => setParts((ps) => [...ps.slice(0, i), p, ...ps.slice(i)]),
                  () => setParts((ps) => ps.filter((_, j) => j !== i))))}
              </HStack>
            );
          })}

          {removed && (
            <HStack justify="space-between" bg="bg.muted" borderRadius="md" px={3}>
              <Text fontSize="sm">Quitaste {removed.name}</Text>
              <Button variant="ghost" minH="44px" onClick={() => { removed.undo(); setRemoved(null); }}>Deshacer</Button>
            </HStack>
          )}
          {mode === 'items' && (
            <Button variant="outline" minH="44px" alignSelf="start"
              onClick={() => edit(() => setRows((rs) => [...rs, { ingredientId: null, quantity: '', unitId: null }]))}>
              <LuPlus /> Agregar insumo
            </Button>
          )}
          {mode === 'package' && (
            <Button variant="outline" minH="44px" alignSelf="start"
              onClick={() => edit(() => setParts((ps) => [...ps, { productId: null, quantity: 1 }]))}>
              <LuPlus /> Agregar producto
            </Button>
          )}
        </VStack>
      </DrawerBody>
      <DrawerFooter gap={2}>
        <Button variant="ghost" minH="44px" onClick={onClose}>Cancelar</Button>
        {!dirty && data.status === 'estimated' && (
          <Button minH="44px" loading={confirm.isPending} onClick={() => confirm.mutate()}>Confirmar</Button>
        )}
        {!dirty && data.status === '' && empty && (
          <Button minH="44px" variant="outline" loading={confirm.isPending} onClick={() => confirm.mutate()}>No lleva nada</Button>
        )}
        {(dirty || data.status !== 'estimated') && (
          <Button minH="44px" disabled={!dirty || !valid} loading={save.isPending} onClick={() => save.mutate()}>Guardar</Button>
        )}
      </DrawerFooter>
    </>
  );
}

function StatusLine({ data }: { data: Composition }) {
  const { zona } = useHoraDelNegocio();
  if (data.status === 'estimated') {
    return <Badge colorPalette="orange" alignSelf="start">Estimado — revísalo y confírmalo</Badge>;
  }
  if (data.status === 'confirmed') {
    return (
      <Text fontSize="sm" color="fg.muted">
        Confirmado{data.confirmedBy ? ` por ${data.confirmedBy}` : ''}
        {data.confirmedAt ? ` el ${soloFecha(data.confirmedAt, zona)}` : ''}
      </Text>
    );
  }
  return <Badge colorPalette="gray" alignSelf="start">Sin capturar</Badge>;
}
