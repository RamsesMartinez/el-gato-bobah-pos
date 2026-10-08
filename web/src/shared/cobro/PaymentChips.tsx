import { Box, Button, HStack, Text } from '@chakra-ui/react';
import type { Currency, PaymentView } from '../../types/pos';
import { money } from '../../utils/format';

interface Props {
  payments: PaymentView[];
  currency: Currency;
  onOpen: (payment: PaymentView) => void;
}

// PaymentChips son los pagos del pedido en una sola fila con scroll horizontal: «Pago 1 · Tarjeta ·
// $110». Vienen del servidor, no de la memoria de la hoja, así que sobreviven a recargar la tableta.
// Un pago devuelto conserva su número y sale tachado: el «Pago 2» impreso sigue siendo el 2.
export function PaymentChips({ payments, currency, onOpen }: Props) {
  if (payments.length === 0) return null;
  return (
    <Box overflowX="auto" mx={-1} px={1}>
      <HStack gap={2} w="max-content">
        {payments.map((p) => (
          <Button key={`${p.id}-${p.voided}`} minH="44px" size="sm" variant="outline"
            colorPalette={p.voided ? 'gray' : 'green'} onClick={() => onOpen(p)}
            aria-label={`Pago ${p.number}${p.voided ? ', devuelto' : ''}`}>
            <Text as="span" textDecoration={p.voided ? 'line-through' : undefined} fontWeight="600">
              Pago {p.number} · {p.methodName} · {money(p.amount, currency)}
            </Text>
            {p.voided && <Text as="span" fontSize="xs" color="fg.muted">Devuelto</Text>}
          </Button>
        ))}
      </HStack>
    </Box>
  );
}
