import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Box, Button, Flex, HStack, IconButton, Input, Table, Text, VStack } from '@chakra-ui/react';
import { LuArrowLeftRight, LuArrowRight, LuMinus, LuPlus, LuSearch } from 'react-icons/lu';
import { Picker } from '../../../components/Picker';
import { NumericKeypad } from '../../../components/NumericKeypad';
import { useKeypadField } from '../../../components/useKeypadField';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../../components/ui/drawer';
import { toaster } from '../../../components/ui/toaster';
import { ApiError } from '../../../api/client';
import { backofficeApi, type StockLevel, type TransferBody, type Warehouse } from '../../../api/backoffice';
import { formatBaseQty } from '../../../shared/units';
import { KEY_REUSED_MESSAGE, toBase, typedUnit } from './transferUnits';

// La pestaña Traspaso (spec 094, maqueta B): «De → A» arriba, lo que hay en el origen con lo que hay en
// el destino al lado, una cantidad por renglón con el teclado de la pantalla, y un solo traspaso con
// todos los renglones. Lo que no está en el origen no se ofrece.

// La clave del borrador vive en la sesión del navegador: un F5 a media confirmación no la cambia, y el
// servidor reconoce el reintento en vez de traspasar dos veces. Se renueva tras un traspaso, al cambiar
// el origen o el destino (es otro traspaso) y cuando el servidor dice que ya se usó.
const KEY_STORAGE = 'egb:traspaso:clave';
function storedKey(): string {
  try {
    const k = sessionStorage.getItem(KEY_STORAGE);
    if (k) return k;
  } catch { /* sin almacenamiento: la clave vive mientras la pantalla siga abierta */ }
  return renewKey();
}
function renewKey(): string {
  const k = crypto.randomUUID();
  try { sessionStorage.setItem(KEY_STORAGE, k); } catch { /* ídem */ }
  return k;
}

type Row = { key: string; itemType: string; id: number; name: string; unit: string; from: string; to: string };
const itemKey = (l: StockLevel) => {
  const id = l.ingredient_id ?? l.product_id;
  return id ? `${l.item_type}:${id}` : null;
};

export function TransferTab({ warehouses }: { warehouses: Warehouse[] }) {
  const qc = useQueryClient();
  const sales = warehouses.find((w) => w.isSales);
  const firstOther = warehouses.find((w) => !w.isSales);
  const [from, setFrom] = useState<number | undefined>(firstOther?.id);
  const [to, setTo] = useState<number | undefined>(sales?.id);
  const [search, setSearch] = useState('');
  const [qty, setQty] = useState<Record<string, string>>({});
  const [note, setNote] = useState('');
  const [clientKey, setClientKey] = useState(storedKey);
  const [error, setError] = useState('');
  const [confirm, setConfirm] = useState<{ key: string; name: string; available: string }[] | null>(null);
  const { field, setField, apply, keep, ref } = useKeypadField<string>();

  const fromLevels = useQuery({ queryKey: ['stock', 'levels', from], queryFn: () => backofficeApi.stockLevels(from), enabled: !!from });
  const toLevels = useQuery({ queryKey: ['stock', 'levels', to], queryFn: () => backofficeApi.stockLevels(to), enabled: !!to });
  const name = (id?: number) => warehouses.find((w) => w.id === id)?.name ?? '';

  const rows = useMemo<Row[]>(() => {
    const dest = new Map<string, string>();
    for (const l of toLevels.data?.items ?? []) {
      const k = itemKey(l);
      if (k) dest.set(k, l.on_hand);
    }
    const out: Row[] = [];
    for (const l of fromLevels.data?.items ?? []) {
      const k = itemKey(l);
      if (!k || Number(l.on_hand) <= 0) continue;
      out.push({ key: k, itemType: l.item_type, id: (l.ingredient_id ?? l.product_id)!, name: l.item_name, unit: l.unit_code,
        from: l.on_hand, to: dest.get(k) ?? '0' });
    }
    return out;
  }, [fromLevels.data, toLevels.data]);
  const q = search.trim().toLowerCase();
  const shown = q ? rows.filter((r) => r.name.toLowerCase().includes(q)) : rows;
  const chosen = rows.filter((r) => toBase(qty[r.key] ?? '', r.unit) > 0);

  const body = (confirmNegative: boolean): TransferBody => ({
    clientKey, fromWarehouseId: from!, toWarehouseId: to!, note: note.trim() || undefined,
    confirmNegative: confirmNegative || undefined,
    lines: chosen.map((r) => ({
      itemType: r.itemType,
      ...(r.itemType === 'producto' ? { productId: r.id } : { ingredientId: r.id }),
      qty: String(toBase(qty[r.key], r.unit)),
    })),
  });
  const send = useMutation({
    mutationFn: (confirmNegative: boolean) => backofficeApi.createTransfer(body(confirmNegative)),
    onSuccess: () => {
      const n = chosen.length;
      toaster.create({ title: `Listo: ${n} ${n === 1 ? 'artículo' : 'artículos'} de ${name(from)} a ${name(to)}`, type: 'success' });
      setQty({});
      setNote('');
      setConfirm(null);
      setField(null);
      setClientKey(renewKey());
      void qc.invalidateQueries({ queryKey: ['stock'] });
      void qc.invalidateQueries({ queryKey: ['warehouses'] });
    },
    onError: (e) => {
      if (e instanceof ApiError && e.code === 'NEEDS_CONFIRM') {
        const byKey = new Map(rows.map((r) => [`${r.itemType}:${r.id}`, r]));
        setConfirm((e.details?.lines ?? []).map((l) => {
          const r = byKey.get(`${l.itemType}:${l.id}`);
          return { key: `${l.itemType}:${l.id}`, name: r?.name ?? '', available: formatBaseQty(l.available, r?.unit ?? '') };
        }));
        return;
      }
      setConfirm(null);
      if (e instanceof ApiError && e.code === 'TRANSFER_KEY_REUSED') {
        setClientKey(renewKey());
        setError(KEY_REUSED_MESSAGE);
        return;
      }
      setError(e instanceof ApiError && e.code === 'WAREHOUSE_NOT_FOUND'
        ? 'Uno de los almacenes ya no existe. Vuelve a elegirlos.'
        : 'No se pudo traspasar. Vuelve a intentar.');
    },
  });

  const setRowQty = (k: string, v: string) => setQty((prev) => ({ ...prev, [k]: v }));
  const step = (r: Row, d: number) => {
    keep();
    const n = Math.max(0, (Number(qty[r.key]) || 0) + d);
    setRowQty(r.key, n === 0 ? '' : String(n));
  };
  // Otro origen u otro destino es otro traspaso: lleva clave nueva.
  const route = (f?: number, t?: number) => {
    setFrom(f);
    setTo(t);
    setQty({});
    setField(null);
    setError('');
    setClientKey(renewKey());
  };
  const options = (exclude?: number) => warehouses.filter((w) => w.id !== exclude)
    .map((w) => ({ value: String(w.id), label: w.isSales ? `${w.name} · de venta` : w.name }));
  const fieldRow = rows.find((r) => r.key === field);

  return (
    <VStack align="stretch" gap={3} h="100%">
      <HStack justify="space-between" flexShrink={0} flexWrap="wrap" gap={3}>
        <HStack gap={2} flexWrap="wrap">
          <Box w="200px" role="group" aria-label="De">
            <Picker title="¿De dónde sale?" value={String(from ?? '')} options={options(to)}
              onChange={(v) => route(Number(v), to)} />
          </Box>
          <LuArrowRight aria-hidden />
          <Box w="200px" role="group" aria-label="A">
            <Picker title="¿A dónde va?" value={String(to ?? '')} options={options(from)} onChange={(v) => route(from, Number(v))} />
          </Box>
          <IconButton aria-label="Invertir" minH="44px" minW="44px" variant="outline"
            onClick={() => route(to, from)}>
            <LuArrowLeftRight />
          </IconButton>
        </HStack>
        <HStack borderWidth="1px" borderRadius="md" px={3} bg="bg.panel" w="260px">
          <LuSearch aria-hidden />
          <Input aria-label="Buscar" minH="44px" border="none" placeholder="Buscar" value={search} onChange={(e) => setSearch(e.target.value)} />
        </HStack>
      </HStack>

      {/* Con 1024 px o más el teclado va a la derecha de la tabla: abajo le quitaba a la lista casi
          todo el alto de la pantalla de 600 px y quedaban uno o dos renglones a la vista. */}
      <Flex flex="1" minH={0} gap={3} direction={{ base: 'column', lg: 'row' }}>
      <Box flex="1" minH={0} overflowY="auto" bg="bg.panel" borderRadius="lg" borderWidth="1px">
        <Table.Root size="sm">
          <Table.Header><Table.Row>
            <Table.ColumnHeader>Artículo</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">En {name(from)}</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">En {name(to)}</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Llevar</Table.ColumnHeader>
          </Table.Row></Table.Header>
          <Table.Body>
            {shown.length === 0 && (
              <Table.Row><Table.Cell colSpan={4}><Text color="fg.subtle">
                {rows.length === 0 ? `${name(from)} no tiene mercancía que llevar` : 'Nada coincide con la búsqueda'}
              </Text></Table.Cell></Table.Row>
            )}
            {shown.map((r) => (
              <Table.Row key={r.key}>
                <Table.Cell>{r.name}</Table.Cell>
                <Table.Cell textAlign="end">{formatBaseQty(r.from, r.unit)}</Table.Cell>
                <Table.Cell textAlign="end" color={Number(r.to) < 0 ? 'fg.error' : 'fg.muted'}>{formatBaseQty(r.to, r.unit)}</Table.Cell>
                <Table.Cell py={1}>
                  <HStack justify="end" gap={1}>
                    <HStack gap={0} borderWidth="1px" borderRadius="md">
                      <IconButton aria-label="Menos" variant="ghost" minH="44px" minW="44px" onClick={() => step(r, -1)}><LuMinus /></IconButton>
                      <Input ref={ref(r.key)} aria-label={`Cuánto de ${r.name}`} w="72px" minH="44px" textAlign="center" border="none"
                        inputMode="none" readOnly fontWeight="700" value={qty[r.key] ?? ''}
                        borderWidth={field === r.key ? '2px' : undefined} borderColor={field === r.key ? 'colorPalette.solid' : undefined}
                        onClick={() => setField(r.key)} onFocus={() => setField(r.key)} />
                      <IconButton aria-label="Más" variant="ghost" minH="44px" minW="44px" onClick={() => step(r, 1)}><LuPlus /></IconButton>
                    </HStack>
                    <Text w="28px">{typedUnit(r.unit).label}</Text>
                  </HStack>
                </Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      </Box>

      {fieldRow && (
        <Flex flexShrink={0} gap={3} align={{ base: 'end', lg: 'stretch' }} direction={{ base: 'row', lg: 'column' }}
          w={{ base: 'auto', lg: '280px' }} data-testid="teclado-traspaso">
          <Box flex={{ base: '1', lg: 'none' }} maxW="320px">
            <NumericKeypad allowDecimal={typedUnit(fieldRow.unit).decimals}
              onKey={(k) => setRowQty(fieldRow.key, apply(qty[fieldRow.key] ?? '', k, typedUnit(fieldRow.unit).decimals))} />
          </Box>
          <Box flex="1" display={{ base: 'block', lg: 'none' }} />
          <Button minH="56px" colorPalette="brand" onClick={() => setField(null)}>Listo</Button>
        </Flex>
      )}
      </Flex>

      {/* Con el teclado abajo (pantalla angosta) el pie se esconde; con el teclado a la derecha se queda. */}
      <VStack align="stretch" gap={1} flexShrink={0} display={fieldRow ? { base: 'none', lg: 'flex' } : 'flex'}>
          <HStack gap={3}>
            <Input aria-label="Nota" placeholder="Nota (opcional)" minH="44px" maxW="420px" maxLength={200}
              value={note} onChange={(e) => setNote(e.target.value)} />
            <Button minH="52px" ml="auto" colorPalette="brand" disabled={chosen.length === 0 || !from || !to} loading={send.isPending}
              onClick={() => { setError(''); send.mutate(false); }}>
              Traspasar {chosen.length} {chosen.length === 1 ? 'artículo' : 'artículos'}
            </Button>
          </HStack>
          {error && <Text fontSize="sm" color="fg.error">{error}</Text>}
      </VStack>

      {confirm && (
        <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) setConfirm(null); }}>
          <DrawerBackdrop />
          <DrawerContent borderTopRadius="2xl">
            <DrawerHeader><Text fontWeight="700">{name(from)} se quedaría en negativo</Text></DrawerHeader>
            <DrawerBody>
              <VStack align="stretch" gap={1}>
                {confirm.map((c) => <Text key={c.key}>{c.name}: hay {c.available} en {name(from)}</Text>)}
                <Text fontSize="sm" color="fg.muted" mt={2}>Si de verdad hay más, el conteo lo corregirá después.</Text>
              </VStack>
            </DrawerBody>
            <DrawerFooter>
              <Button minH="52px" variant="outline" onClick={() => setConfirm(null)}>Revisar cantidades</Button>
              <Button minH="52px" ml="auto" colorPalette="orange" loading={send.isPending} onClick={() => send.mutate(true)}>
                Traspasar de todos modos
              </Button>
            </DrawerFooter>
          </DrawerContent>
        </DrawerRoot>
      )}
    </VStack>
  );
}
