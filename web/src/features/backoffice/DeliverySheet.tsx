import { useState } from 'react';
import { Box, Button, HStack, IconButton, Input, Text, VStack } from '@chakra-ui/react';
import { LuMinus, LuPlus } from 'react-icons/lu';
import { useQuery } from '@tanstack/react-query';

import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { DatePickerButton } from '../../components/DatePickerButton';
import { NumericKeypad } from '../../components/NumericKeypad';
import { useKeypadField } from '../../components/useKeypadField';
import { backofficeApi, type DeliveryBody, type EditResult, type ExpenseDetail, type ExpenseItem } from '../../api/backoffice';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { medirAccion } from '../../api/uso';
import { parseNumero, round2 } from '../../domain/numeros';
import { EditPreviewSheet } from './EditPreviewSheet';
import { missingOf, pendingGoods } from './expenseEdit';

// Registrar una entrega (spec 053, maqueta «Hoja de entrega», opción B + «No llegó»). Cada renglón
// pendiente trae lo que falta ya puesto: el pedido que llega entero sale sin tocar nada. «No llegó»
// deja un renglón en 0. La cantidad se escribe con el teclado en pantalla (pedido del dueño): el del
// sistema tapaba «Ver efectos» en la tableta.

const num = (s: string | null | undefined) => {
  const n = parseNumero(s ?? '');
  return n.estado === 'valido' ? n.valor : 0;
};

// unitWord: «paquetes», «pieza», «kg».
function unitWord(i: ExpenseItem, n: number): string {
  if (i.presentationId) return n === 1 ? 'empaque' : 'empaques';
  if (i.unitCode === 'pieza') return n === 1 ? 'pieza' : 'piezas';
  return i.unitCode ?? '';
}

type FieldKey = `${'qty' | 'extra'}:${number}`;

export function DeliverySheet({ expense, onClose, onDone, onlyItemId }: {
  expense: ExpenseDetail;
  onClose: () => void;
  onDone: (r: EditResult) => void;
  // onlyItemId: se abrió desde un renglón («Por recibir»): solo ése viene precargado.
  onlyItemId?: number;
}) {
  const hora = useHoraDelNegocio();
  const today = hora.diaDelNegocio(new Date());
  const lines = pendingGoods(expense);
  const [day, setDay] = useState(today);
  const [perLine, setPerLine] = useState(false);
  const [days, setDays] = useState<Record<number, string>>({});
  const [qty, setQty] = useState<Record<number, string>>(() => Object.fromEntries(lines.map((l) =>
    [l.id, onlyItemId === undefined || onlyItemId === l.id ? String(missingOf(l)) : '0'])));
  const [extra, setExtra] = useState<Record<number, string>>({});
  const { field: activeKey, setField: setActive, apply, ref } = useKeypadField<FieldKey>();
  const field = activeKey ? { kind: activeKey.split(':')[0] as 'qty' | 'extra', id: Number(activeKey.split(':')[1]) } : null;
  const setField = (f: { id: number; kind: 'qty' | 'extra' } | null) => setActive(f ? `${f.kind}:${f.id}` : null);
  const [reviewing, setReviewing] = useState(false);

  const arrived = lines.filter((l) => num(qty[l.id]) > 0);
  const over = (l: ExpenseItem) => !l.isGift && num(qty[l.id]) > missingOf(l);
  const ready = arrived.length > 0 && arrived.every((l) => !over(l) || num(extra[l.id]) > 0);
  const allowDecimal = (l: ExpenseItem) => !l.presentationId && l.unitCode !== 'pieza';

  const key = (k: string) => {
    if (!field) return;
    const l = lines.find((x) => x.id === field.id);
    if (!l) return;
    if (field.kind === 'qty') { const v = apply(qty[l.id] ?? '', k, allowDecimal(l)); setQty((p) => ({ ...p, [l.id]: v })); }
    else { const v = apply(extra[l.id] ?? '', k, true); setExtra((p) => ({ ...p, [l.id]: v })); }
  };
  const bump = (l: ExpenseItem, d: 1 | -1) => setQty((p) => ({ ...p, [l.id]: String(round2(Math.max(0, num(p[l.id]) + d))) }));

  const body = (decisions: Record<string, string>, fingerprint?: string): DeliveryBody => ({
    on: day,
    lines: arrived.map((l) => {
      const short = decisions[`shortfall:${l.id}`];
      return {
        itemId: l.id, qty: String(num(qty[l.id])),
        ...(perLine && days[l.id] && days[l.id] !== day ? { on: days[l.id] } : {}),
        ...(short === 'pending' || short === 'adjust' ? { shortfall: short } : {}),
        ...(over(l) ? { extraAmount: String(num(extra[l.id])) } : {}),
      };
    }),
    decisions, fingerprint,
  });

  return (
    <>
      <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
        <DrawerBackdrop />
        <DrawerContent borderTopRadius="2xl" maxH="90dvh">
          <DrawerHeader>
            <HStack gap={3} flexWrap="wrap">
              <Text fontWeight="700" fontSize="lg" flex="1">Recibir · Gasto {expense.id}</Text>
              <Text fontSize="sm" color="fg.muted">Llegó el</Text>
              <DatePickerButton value={day} onChange={setDay} max={today} label="Día de la entrega" />
              <Button minH="44px" variant="ghost" onClick={() => setPerLine((v) => !v)}>
                {perLine ? 'Una fecha para todo' : 'Fechas por renglón'}
              </Button>
            </HStack>
          </DrawerHeader>
          <DrawerBody>
            <VStack align="stretch" gap={2}>
              {lines.map((l) => {
                const n = num(qty[l.id]);
                const missing = missingOf(l);
                const left = round2(missing - n);
                const name = l.itemName ?? l.description;
                const active = (k: 'qty' | 'extra') => field?.id === l.id && field.kind === k;
                return (
                  <HStack key={l.id} borderWidth="1px" borderRadius="lg" px={3} py={2} gap={3} minH="60px" flexWrap="wrap"
                    borderColor={n > 0 ? 'green.muted' : undefined} bg={n > 0 ? 'green.subtle' : undefined} data-testid="renglon-entrega">
                    <HStack flex="1" minW="200px" gap={3} flexWrap="wrap">
                      <Text fontWeight="600">{name}</Text>
                      <Text fontSize="sm" color="fg.muted">Falta{missing === 1 ? '' : 'n'} {missing} {unitWord(l, missing)}</Text>
                      {n > 0 && left > 0 && (
                        <Text fontSize="sm" fontWeight="600" color="fg.warning">⚠ {left === 1 ? 'Faltará' : 'Faltarán'} {left} {unitWord(l, left)}</Text>
                      )}
                    </HStack>
                    {perLine && (
                      <DatePickerButton value={days[l.id] ?? day} onChange={(v) => setDays((p) => ({ ...p, [l.id]: v }))}
                        max={today} label={`Llegó el día de ${name}`} />
                    )}
                    <HStack gap={0} borderWidth="1px" borderRadius="md">
                      <IconButton aria-label={`Menos ${name}`} variant="ghost" minH="44px" minW="44px" onClick={() => bump(l, -1)}><LuMinus /></IconButton>
                      <Input ref={ref(`qty:${l.id}`)} aria-label={`Cuántos llegaron de ${name}`} w="72px" minH="44px" textAlign="center" fontWeight="700"
                        inputMode="none" readOnly value={qty[l.id] ?? ''}
                        borderColor={active('qty') ? 'brand.solid' : 'transparent'} borderWidth="2px"
                        onClick={() => setField({ id: l.id, kind: 'qty' })} onFocus={() => setField({ id: l.id, kind: 'qty' })} />
                      <IconButton aria-label={`Más ${name}`} variant="ghost" minH="44px" minW="44px" onClick={() => bump(l, 1)}><LuPlus /></IconButton>
                    </HStack>
                    {n > 0
                      ? <Button minH="44px" variant="outline" onClick={() => { setQty((p) => ({ ...p, [l.id]: '0' })); setField(null); }}>No llegó</Button>
                      : <Button minH="44px" variant="ghost" onClick={() => setQty((p) => ({ ...p, [l.id]: String(missing) }))}>Llegó todo</Button>}
                    {over(l) && (
                      <HStack w="100%" gap={2}>
                        <Text fontSize="sm">Llegó de más. ¿Cuánto pagaste por lo de más?</Text>
                        <Text fontWeight="700">$</Text>
                        <Input ref={ref(`extra:${l.id}`)} aria-label={`Importe de lo de más de ${name}`} w="110px" minH="44px" inputMode="none" readOnly
                          value={extra[l.id] ?? ''} borderColor={active('extra') ? 'brand.solid' : undefined} borderWidth="2px"
                          onClick={() => setField({ id: l.id, kind: 'extra' })} onFocus={() => setField({ id: l.id, kind: 'extra' })} />
                      </HStack>
                    )}
                  </HStack>
                );
              })}
            </VStack>
          </DrawerBody>
          <DrawerFooter>
            {field ? (
              <HStack w="100%" gap={3} align="end">
                <Box flex="1" maxW="320px">
                  <NumericKeypad allowDecimal={field.kind === 'extra' || allowDecimal(lines.find((l) => l.id === field.id)!)} onKey={key} />
                </Box>
                <Box flex="1" />
                <Button minH="56px" colorPalette="brand" onClick={() => setField(null)}>Listo</Button>
              </HStack>
            ) : (
              <HStack w="100%" gap={3}>
                <Text fontSize="sm" color="fg.muted">
                  {arrived.length === 0 ? 'Nada llegó todavía' : `${arrived.length} de ${lines.length} renglones`}
                </Text>
                <Box flex="1" />
                <Button minH="44px" variant="outline" onClick={onClose}>Cancelar</Button>
                <Button minH="44px" colorPalette="brand" disabled={!ready} onClick={() => setReviewing(true)}>Ver efectos</Button>
              </HStack>
            )}
          </DrawerFooter>
        </DrawerContent>
      </DrawerRoot>
      {reviewing && (
        <EditPreviewSheet title="Antes de recibir"
          request={(preview, decisions, fingerprint) => backofficeApi.deliverExpense(expense.id, body(decisions, fingerprint), preview)}
          onClose={() => setReviewing(false)}
          onDone={(r) => { medirAccion('gastos', 'recibir-entrega'); setReviewing(false); onDone(r); }} />
      )}
    </>
  );
}

// DeliveryForExpense abre la hoja de entrega de un gasto desde fuera de su detalle (el atajo
// «Recibir» de la lista de gastos y de «Por recibir»): primero trae el gasto.
export function DeliveryForExpense({ expenseId, onClose, onDone, onlyItemId }: {
  expenseId: number; onClose: () => void; onDone: (r: EditResult) => void; onlyItemId?: number;
}) {
  const { data } = useQuery({ queryKey: ['expense', expenseId], queryFn: () => backofficeApi.expenseDetail(expenseId) });
  if (!data) return null;
  return <DeliverySheet expense={data} onClose={onClose} onDone={onDone} onlyItemId={onlyItemId} />;
}
