import { useMemo, useState } from 'react';
import { Badge, Box, Button, HStack, IconButton, Input, Text, VStack } from '@chakra-ui/react';
import { LuPlus, LuTrash2 } from 'react-icons/lu';
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

function Editor({ kind, id, data, onClose }: { kind: CompositionKind; id: number; data: Composition; onClose: () => void }) {
  const qc = useQueryClient();
  const [mode, setMode] = useState<'items' | 'product'>(data.linkedProductId ? 'product' : 'items');
  const [rows, setRows] = useState<Row[]>(() => (data.items ?? []).map((it) => ({
    ingredientId: it.ingredientId, quantity: it.quantity, unitId: it.unitId,
  })));
  const [linked, setLinked] = useState<number | null>(data.linkedProductId ?? null);
  const [dirty, setDirty] = useState(false);
  // El último renglón quitado, para deshacerlo con un toque: quitar es un toque y recapturar son
  // varios (insumo, cantidad, unidad).
  const [removed, setRemoved] = useState<{ row: Row; index: number; name: string } | null>(null);

  const ingredients = useQuery({ queryKey: ['ingredients', 'all'], queryFn: () => backofficeApi.ingredients(), enabled: data.editable });
  const units = useQuery({ queryKey: ['units'], queryFn: () => backofficeApi.units(), enabled: data.editable });
  const products = useQuery({
    queryKey: ['admin', 'products', 'linkable'],
    queryFn: () => adminApi.products({ status: 'act', limit: 0 }),
    enabled: data.editable && kind === 'option',
  });

  const ingById = useMemo(() => new Map((ingredients.data?.items ?? []).map((i) => [i.id, i])), [ingredients.data]);
  // Los que ya trae la composición, aunque estén inactivos: si no, el renglón se vería vacío.
  const ingOptions = useMemo(() => {
    const opts = (ingredients.data?.items ?? []).map((i) => ({ value: String(i.id), label: i.name }));
    for (const it of data.items ?? []) {
      if (!ingById.has(it.ingredientId)) opts.push({ value: String(it.ingredientId), label: it.ingredientName });
    }
    return opts;
  }, [ingredients.data, ingById, data.items]);

  const edit = (fn: () => void) => { fn(); setDirty(true); };
  const done = (c: Composition, title: string) => {
    qc.setQueryData(['admin', 'composition', kind, id], c);
    qc.invalidateQueries({ queryKey: ['admin', kind === 'product' ? 'products' : 'modifier-options'] });
    toaster.create({ title, type: 'success' });
    onClose();
  };
  const fail = (e: unknown) => toaster.create({ title: 'No se guardó', description: String(e), type: 'error' });

  const save = useMutation({
    mutationFn: () => adminApi.saveComposition(kind, id, mode === 'product'
      ? { items: [], linkedProductId: linked }
      : {
        items: rows.map((r) => ({ ingredientId: r.ingredientId!, quantity: r.quantity, unitId: r.unitId! })),
        linkedProductId: null,
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
          <Text>
            {data.reason === 'package'
              ? 'Este paquete descuenta los productos que lleva.'
              : 'Este producto descuenta sus propias existencias.'}
          </Text>
        </DrawerBody>
        <DrawerFooter><Button minH="44px" onClick={onClose}>Cerrar</Button></DrawerFooter>
      </>
    );
  }

  const valid = mode === 'product'
    ? linked !== null
    : rows.every((r) => r.ingredientId !== null && r.unitId !== null && Number(r.quantity) > 0);
  const empty = mode === 'items' && rows.length === 0;

  return (
    <>
      <DrawerBody>
        <VStack align="stretch" gap={3}>
          <StatusLine data={data} />
          {kind === 'option' && (
            <HStack gap={2}>
              <Button flex="1" minH="44px" variant={mode === 'items' ? 'solid' : 'outline'}
                onClick={() => edit(() => setMode('items'))}>Lleva insumos</Button>
              <Button flex="1" minH="44px" variant={mode === 'product' ? 'solid' : 'outline'}
                onClick={() => edit(() => setMode('product'))}>Es un producto</Button>
            </HStack>
          )}
          {mode === 'product' ? (
            <Picker title="Producto" placeholder="Elegir producto"
              value={linked ? String(linked) : ''}
              options={(products.data?.items ?? []).map((p) => ({ value: String(p.id), label: p.name }))
                .concat(linked && data.linkedProductName && !(products.data?.items ?? []).some((p) => p.id === linked)
                  ? [{ value: String(linked), label: data.linkedProductName }] : [])}
              onChange={(v) => edit(() => setLinked(Number(v)))} />
          ) : (
            <>
              {rows.map((r, i) => {
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
                    {/* Separado de la unidad, que es lo último que se toca en cada renglón, y con
                        «Deshacer» abajo: quitar por error obliga a recapturar el renglón entero. */}
                    <IconButton aria-label={`Quitar ${ingName}`} variant="ghost" colorPalette="red" minH="44px" minW="44px" ml={3}
                      onClick={() => edit(() => {
                        setRemoved({ row: r, index: i, name: ingName });
                        setRows((rs) => rs.filter((_, j) => j !== i));
                      })}>
                      <LuTrash2 />
                    </IconButton>
                  </HStack>
                );
              })}
              {removed && (
                <HStack justify="space-between" bg="bg.muted" borderRadius="md" px={3}>
                  <Text fontSize="sm">Quitaste {removed.name || 'un renglón'}</Text>
                  <Button variant="ghost" minH="44px" onClick={() => {
                    const back = removed;
                    setRows((rs) => [...rs.slice(0, back.index), back.row, ...rs.slice(back.index)]);
                    setRemoved(null);
                  }}>Deshacer</Button>
                </HStack>
              )}
              <Button variant="outline" minH="44px" alignSelf="start"
                onClick={() => edit(() => setRows((rs) => [...rs, { ingredientId: null, quantity: '', unitId: null }]))}>
                <LuPlus /> Agregar insumo
              </Button>
            </>
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
