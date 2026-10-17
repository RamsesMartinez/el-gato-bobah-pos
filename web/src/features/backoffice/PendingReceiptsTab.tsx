import { useEffect, useState } from 'react';
import { medirAccion } from '../../api/uso';
import { Box, Button, HStack, Input, Text, VStack, Center, Spinner } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { backofficeApi, type PendingReceipt } from '../../api/backoffice';
import { money, normalize } from '../../utils/format';
import { parseMonto, parseNumero } from '../../domain/numeros';
import { formatBaseQty } from '../../shared/units';
import { ArrivedItemSheet, ReceiptSheet } from './ArrivedItemSheet';
import { receiptError } from './receiptError';
import { baseOfLine } from './expenseDraft';

// «Por recibir» (spec 038, maqueta «Almacén › Por recibir», opción A con buscador): lo pagado que
// no ha llegado, agrupado por compra. Se recibe por partes con la cantidad tecleada, de más con su
// importe, se cierra lo que ya no va a llegar, y «Llegó algo más» agrega lo que no venía.

const MESES = ['ene', 'feb', 'mar', 'abr', 'may', 'jun', 'jul', 'ago', 'sep', 'oct', 'nov', 'dic'];
const dia = (iso: string) => { const [, m, d] = iso.split('-').map(Number); return `${d} ${MESES[m - 1]}`; };
const num = (s: string | null | undefined) => { const n = parseNumero(s ?? ''); return n.estado === 'valido' ? n.valor : NaN; };

// unitWord dice la unidad del renglón para una cantidad: «cajas de 12 L», «caja de 12 L», «kg».
function unitWord(r: Pick<PendingReceipt, 'presentationName' | 'packQtyInBase' | 'baseUnitCode' | 'unitCode'>, n: number): string {
  if (r.presentationName && r.packQtyInBase) {
    const name = r.presentationName.toLowerCase();
    return `${n === 1 ? name : `${name}s`} de ${formatBaseQty(r.packQtyInBase, r.baseUnitCode ?? '')}`;
  }
  return r.unitCode === 'pieza' ? (n === 1 ? 'pieza' : 'piezas') : (r.unitCode ?? '');
}

const qty = (s: string) => String(Number(Number(s).toFixed(4)));

type Action =
  | { kind: 'receive'; row: PendingReceipt }
  | { kind: 'notComing'; row: PendingReceipt }
  | { kind: 'all'; rows: PendingReceipt[] }
  | { kind: 'more'; rows: PendingReceipt[] };

export function PendingReceiptsTab() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ['stock', 'pending-receipts'], queryFn: backofficeApi.pendingReceipts });
  const [q, setQ] = useState('');
  const [action, setAction] = useState<Action | null>(null);
  const [done, setDone] = useState('');
  useEffect(() => { medirAccion('inventario', 'ver-por-recibir'); }, []);

  const refresh = (msg: string) => {
    setAction(null);
    setDone(msg);
    void qc.invalidateQueries({ queryKey: ['stock'] });
    void qc.invalidateQueries({ queryKey: ['expenses'] });
    void qc.invalidateQueries({ queryKey: ['expense'] });
  };

  if (isLoading) return <Center py={16}><Spinner size="xl" /></Center>;
  const rows = data ?? [];
  const needle = normalize(q.trim());
  const shown = needle
    ? rows.filter((r) => normalize(r.itemName).includes(needle) || normalize(r.supplier ?? '').includes(needle))
    : rows;
  const groups = new Map<number, PendingReceipt[]>();
  for (const r of shown) groups.set(r.expenseId, [...(groups.get(r.expenseId) ?? []), r]);

  return (
    <VStack align="stretch" gap={3}>
      <Input aria-label="Buscar" placeholder="Buscar artículo o proveedor…" minH="44px" value={q} onChange={(e) => setQ(e.target.value)} />
      {done && <Text p={3} borderRadius="lg" bg="green.subtle" fontSize="sm" data-testid="listo">{done}</Text>}
      {rows.length === 0 && <Text color="fg.muted" py={6} textAlign="center">No hay nada por recibir.</Text>}
      {rows.length > 0 && shown.length === 0 && <Text color="fg.muted" py={6} textAlign="center">Nada coincide con «{q}».</Text>}
      {[...groups.values()].map((g) => {
        const head = g[0];
        return (
          <Box key={head.expenseId} borderWidth="1px" borderRadius="lg" bg="bg.panel" data-testid="compra-por-recibir">
            <HStack p={3} gap={3} borderBottomWidth="1px" flexWrap="wrap">
              <Text fontWeight="700" flex="1">{head.supplier ?? 'Sin proveedor'}</Text>
              <Text fontSize="sm" color="fg.muted">
                compra del {dia(head.expenseDate)} · {head.paid ? 'pagado' : 'por pagar'} {money(head.expenseAmount)}
              </Text>
              <Button minH="44px" variant="outline" onClick={() => setAction({ kind: 'more', rows: g })}>Llegó algo más…</Button>
              <Button minH="44px" variant="outline" onClick={() => setAction({ kind: 'all', rows: g })}>Llegó todo…</Button>
            </HStack>
            {g.map((r) => {
              const got = num(r.received);
              return (
                <HStack key={r.itemId} p={3} gap={3} flexWrap="wrap" data-testid="renglon-por-recibir">
                  <VStack align="start" gap={0} flex="1" minW="200px">
                    <Text fontWeight="600">{r.itemName}</Text>
                    <Text fontSize="xs" color="fg.muted">
                      pediste {qty(r.quantity)} {unitWord(r, num(r.quantity))}
                      {got > 0 && ` · ya llegaron ${qty(r.received)}`}
                      {num(r.notComing) > 0 && ` · ${qty(r.notComing)} ya no llegan`}
                    </Text>
                  </VStack>
                  <VStack align="start" gap={0} minW="120px">
                    <Text fontSize="xs" color="fg.muted">Falta</Text>
                    <Text fontWeight="700">{qty(r.missing)} {unitWord(r, num(r.missing))}</Text>
                  </VStack>
                  <Button minH="44px" variant="outline" onClick={() => setAction({ kind: 'receive', row: r })}>Recibir…</Button>
                  <Button minH="44px" variant="outline" onClick={() => setAction({ kind: 'notComing', row: r })}>Ya no va a llegar…</Button>
                </HStack>
              );
            })}
          </Box>
        );
      })}

      {action?.kind === 'receive' && <ReceiveSheet row={action.row} onClose={() => setAction(null)} onDone={refresh} />}
      {action?.kind === 'notComing' && <NotComingSheet row={action.row} onClose={() => setAction(null)} onDone={refresh} />}
      {action?.kind === 'all' && <AllArrivedSheet rows={action.rows} onClose={() => setAction(null)} onDone={refresh} />}
      {action?.kind === 'more' && <ArrivedItemSheet expenseId={action.rows[0].expenseId} supplier={action.rows[0].supplier} onClose={() => setAction(null)} onDone={refresh} />}
    </VStack>
  );
}


function ReceiveSheet({ row, onClose, onDone }: { row: PendingReceipt; onClose: () => void; onDone: (m: string) => void }) {
  const [value, setValue] = useState(''); // vacía al abrir: se teclea lo que llegó, no se confirma lo pedido
  const [extra, setExtra] = useState('');
  const n = num(value);
  const missing = num(row.missing);
  const over = n > missing ? Number((n - missing).toFixed(4)) : 0;
  const extraMoney = parseMonto(extra);
  const factor = row.packQtyInBase ?? (row.unitCode === row.baseUnitCode ? '1' : null);
  const base = Number.isFinite(n) && n > 0 ? baseOfLine(value, factor) : null;
  const ok = Number.isFinite(n) && n > 0 && (over === 0 || (extraMoney.estado === 'valido' && extraMoney.valor > 0));
  const save = useMutation({
    mutationFn: () => backofficeApi.receiveItem(row.expenseId, row.itemId, {
      qty: value, seenApplied: row.applied, extraAmount: over > 0 ? extra : undefined,
    }),
    onSuccess: () => { medirAccion('inventario', 'recibir'); onDone(`Listo. ${row.itemName}: llegaron ${qty(value)} ${unitWord(row, n)}.`); },
  });
  return (
    <ReceiptSheet title={`¿Cuánto llegó de ${row.itemName}?`} onClose={onClose}
      footer={<Button minH="44px" colorPalette="red" disabled={!ok} loading={save.isPending} onClick={() => save.mutate()}>Guardar</Button>}>
      <VStack align="stretch" gap={3}>
        <Text fontSize="sm" color="fg.muted">{row.supplier ?? 'Sin proveedor'} · compra del {dia(row.expenseDate)} · faltan {qty(row.missing)}</Text>
        <HStack gap={3}>
          <Text>Llegaron</Text>
          <Input aria-label="Llegaron" w="100px" minH="44px" inputMode="decimal" value={value} onChange={(e) => setValue(e.target.value)} />
          <Text>{unitWord(row, Number.isFinite(n) ? n : 2)}</Text>
          {base !== null && <Text color="green.700" fontWeight="700">= {formatBaseQty(base, row.baseUnitCode ?? '')}</Text>}
        </HStack>
        {over > 0 && (
          <HStack gap={2} p={3} borderRadius="lg" bg="orange.subtle" flexWrap="wrap">
            <Text fontSize="sm">Llegaron {qty(String(over))} de más. ¿Cuánto pagaste por lo de más?</Text>
            <HStack gap={1}>
              <Text fontWeight="700">$</Text>
              <Input aria-label="Importe de lo de más" w="120px" minH="44px" inputMode="decimal" value={extra} onChange={(e) => setExtra(e.target.value)} />
            </HStack>
            <Text fontSize="sm">Sube el total pagado de esta compra.</Text>
          </HStack>
        )}
        {save.isError && <Text color="red.600" fontSize="sm">{receiptError(save.error)}</Text>}
      </VStack>
    </ReceiptSheet>
  );
}

function NotComingSheet({ row, onClose, onDone }: { row: PendingReceipt; onClose: () => void; onDone: (m: string) => void }) {
  const [value, setValue] = useState('');
  const n = num(value);
  const missing = num(row.missing);
  const ok = Number.isFinite(n) && n > 0 && n <= missing;
  const save = useMutation({
    mutationFn: () => backofficeApi.markNotComing(row.expenseId, row.itemId, value),
    onSuccess: () => onDone(`Listo. ${qty(value)} ${unitWord(row, n)} de ${row.itemName} ya no van a llegar. Lo pagado no cambia.`),
  });
  return (
    <ReceiptSheet title={`¿Cuántas ya no van a llegar de ${row.itemName}?`} onClose={onClose}
      footer={<Button minH="44px" colorPalette="red" disabled={!ok} loading={save.isPending} onClick={() => save.mutate()}>Guardar</Button>}>
      <VStack align="stretch" gap={3}>
        <HStack gap={3}>
          <Input aria-label="Ya no llegan" w="100px" minH="44px" inputMode="decimal" value={value} onChange={(e) => setValue(e.target.value)} />
          <Text>de {qty(row.missing)} {unitWord(row, missing)} que faltan</Text>
        </HStack>
        {Number.isFinite(n) && n > missing && <Text color="orange.700" fontSize="sm">Solo faltan {qty(row.missing)}.</Text>}
        <Text fontSize="sm" color="fg.muted">No suma a lo guardado ni cambia lo pagado.</Text>
        {save.isError && <Text color="red.600" fontSize="sm">{receiptError(save.error)}</Text>}
      </VStack>
    </ReceiptSheet>
  );
}

function AllArrivedSheet({ rows, onClose, onDone }: { rows: PendingReceipt[]; onClose: () => void; onDone: (m: string) => void }) {
  const head = rows[0];
  const save = useMutation({
    mutationFn: () => backofficeApi.receiveExpense(head.expenseId, {}),
    onSuccess: () => onDone(`Listo. Llegó todo lo de ${head.supplier ?? 'la compra'}.`),
  });
  return (
    <ReceiptSheet title={`¿Llegó todo de ${head.supplier ?? 'esta compra'}?`} onClose={onClose}
      footer={<Button minH="44px" colorPalette="red" loading={save.isPending} onClick={() => save.mutate()}>Sí, llegó todo</Button>}>
      <VStack align="stretch" gap={1}>
        {rows.map((r) => (
          <Text key={r.itemId}>{r.itemName}: {qty(r.missing)} {unitWord(r, num(r.missing))}</Text>
        ))}
        {save.isError && <Text color="red.600" fontSize="sm">{receiptError(save.error)}</Text>}
      </VStack>
    </ReceiptSheet>
  );
}

