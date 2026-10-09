import { useState, type ReactNode } from 'react';
import { Box, Button, HStack, SimpleGrid, Text, VStack } from '@chakra-ui/react';
import { LuCheck, LuChevronDown, LuChevronRight, LuMinus, LuPlus } from 'react-icons/lu';
import type { Currency, PaymentView } from '../../types/pos';
import { money } from '../../utils/format';
import type { ListOrder, ListRowState } from './listOrder';

interface Props {
  rows: ListOrder;
  // Cuántas piezas de cada renglón paga esta persona. Un renglón ausente no está elegido.
  selection: Record<number, number>;
  payments: PaymentView[];
  currency: Currency;
  disabled?: boolean;
  // «Todo lo que falta» y «Pasar a otro pedido», en el renglón del título: abajo, en el pie,
  // empujaban los billetes fuera de la vista a 600 px.
  acciones?: ReactNode;
  onToggle: (row: ListRowState) => void;
  onQty: (lineId: number, qty: number) => void;
}

// ByProducts es la lista de «Toca lo que paga esta persona». Lo pendiente arriba con casillas
// grandes en dos columnas; lo pagado al final, en gris y con su pago, y agrupado si son muchos.
//
// No calcula ningún monto: el de la selección lo da el servidor (quote). Aquí solo se elige qué.
export function ByProducts({ rows, selection, payments, currency, disabled, acciones, onToggle, onQty }: Props) {
  const [showPaid, setShowPaid] = useState(false);
  const methodOf = new Map(payments.filter((p) => !p.voided).map((p) => [p.number, p.methodName]));
  return (
    <VStack align="stretch" gap={2}>
      <HStack justify="space-between" gap={2} flexWrap="wrap">
        <Text fontSize="sm" fontWeight="600">Toca lo que paga esta persona</Text>
        {acciones}
      </HStack>
      <SimpleGrid columns={2} gap={2}>
        {rows.pending.map((r) => {
          const picked = selection[r.line.id] ?? 0;
          const on = picked > 0;
          const unit = Number(r.line.lineTotal) / Number(r.line.quantity);
          return (
            <Box key={r.line.id} borderWidth="1px" borderRadius="lg"
              borderColor={on ? 'colorPalette.solid' : 'border'} bg={on ? 'colorPalette.subtle' : undefined}>
              <Button variant="ghost" w="100%" minH="48px" h="auto" py={2} px={2} justifyContent="flex-start"
                disabled={disabled} aria-pressed={on} onClick={() => onToggle(r)}
                aria-label={`${r.line.productName}${on ? ', elegido' : ''}`}>
                <HStack w="100%" gap={2} align="start">
                  <Box boxSize="28px" flexShrink={0} borderWidth="2px" borderRadius="md" display="grid" placeItems="center"
                    borderColor={on ? 'colorPalette.solid' : 'border.emphasized'} bg={on ? 'colorPalette.solid' : undefined}
                    color="colorPalette.contrast">
                    {on && <LuCheck size={18} />}
                  </Box>
                  <VStack align="start" gap={0} flex="1" minW={0}>
                    <Text fontWeight="600" lineClamp={2} textAlign="left" whiteSpace="normal">
                      {r.free > 1 ? `${r.free}× ` : ''}{r.line.productName}
                    </Text>
                    {r.line.modifiers && r.line.modifiers.length > 0 && (
                      <Text fontSize="xs" color="fg.muted" lineClamp={1} textAlign="left" whiteSpace="normal">
                        {r.line.modifiers.map((m) => m.name).join(' · ')}
                      </Text>
                    )}
                    {r.paid > 0 && (
                      <Text fontSize="xs" color="fg.muted">{r.paid} pagado{r.paid > 1 ? 's' : ''}</Text>
                    )}
                  </VStack>
                  <Text fontWeight="700" flexShrink={0}>
                    {r.free > 1 ? `${money(String(unit), currency)} c/u` : money(String(unit), currency)}
                  </Text>
                </HStack>
              </Button>
              {on && r.free > 1 && (
                <HStack justify="center" gap={2} pb={2}>
                  <Button aria-label={`Una ${r.line.productName} menos`} minH="44px" minW="44px" variant="outline"
                    colorPalette="gray" disabled={disabled || picked <= 1} onClick={() => onQty(r.line.id, picked - 1)}>
                    <LuMinus />
                  </Button>
                  <Text fontWeight="700" minW="4.5rem" textAlign="center">{picked} de {r.free}</Text>
                  <Button aria-label={`Una ${r.line.productName} más`} minH="44px" minW="44px" variant="outline"
                    colorPalette="gray" disabled={disabled || picked >= r.free} onClick={() => onQty(r.line.id, picked + 1)}>
                    <LuPlus />
                  </Button>
                </HStack>
              )}
            </Box>
          );
        })}
      </SimpleGrid>
      {rows.paid.length > 0 && rows.groupPaid && (
        <Button minH="44px" variant="ghost" colorPalette="gray" justifyContent="flex-start"
          onClick={() => setShowPaid((v) => !v)}>
          {showPaid ? <LuChevronDown /> : <LuChevronRight />} {rows.paid.length} pagados
        </Button>
      )}
      {rows.paid.length > 0 && (!rows.groupPaid || showPaid) && (
        <SimpleGrid columns={2} gap={2}>
          {rows.paid.map((r) => (
            <HStack key={r.line.id} px={3} py={2} borderRadius="lg" bg="bg.muted" color="fg.muted" gap={2} minH="44px">
              <VStack align="start" gap={0} flex="1" minW={0}>
                <Text fontWeight="600" lineClamp={1}>
                  {Number(r.line.quantity) > 1 ? `${r.line.quantity}× ` : ''}{r.line.productName}
                </Text>
                <Text fontSize="xs">
                  {r.paidBy.map((n) => `Pago ${n} · ${methodOf.get(n) ?? ''}`).join(', ')}
                </Text>
              </VStack>
              <Text fontWeight="600">{money(r.line.lineTotal, currency)}</Text>
            </HStack>
          ))}
        </SimpleGrid>
      )}
    </VStack>
  );
}
