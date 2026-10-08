import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { LuMinus, LuPlus } from 'react-icons/lu';
import type { Currency, PaymentView } from '../../types/pos';
import { money } from '../../utils/format';
import { MAX_PEOPLE, nextPart } from './split';

interface Props {
  of: number;
  payments: PaymentView[];
  // El monto de la parte que se cobra ahora, de la cotización. Nulo mientras llega.
  currentAmount: string | null;
  currency: Currency;
  outstanding: string;
  disabled?: boolean;
  onChange: (of: number) => void;
}

// EvenSplit reparte lo que falta entre personas. El monto de cada parte lo calcula el servidor sobre
// lo que falta en ese momento —entre una parte y otra pudo entrar otro pago— y la última se lleva el
// centavo. Aquí solo se elige entre cuántos.
export function EvenSplit({ of, payments, currentAmount, currency, outstanding, disabled, onChange }: Props) {
  const current = nextPart(of, payments);
  const paidBefore = payments.some((p) => !p.voided && p.split?.of !== of);
  return (
    <VStack align="stretch" gap={2}>
      <HStack justify="space-between" gap={2}>
        <Text fontSize="sm" fontWeight="600">Lo que falta entre</Text>
        <HStack gap={2}>
          <Button aria-label="Una persona menos" minH="44px" minW="44px" variant="outline" colorPalette="gray"
            disabled={disabled || of <= 2} onClick={() => onChange(of - 1)}>
            <LuMinus />
          </Button>
          <Text fontWeight="800" fontSize="lg" minW="6.5rem" textAlign="center">{of} personas</Text>
          <Button aria-label="Una persona más" minH="44px" minW="44px" variant="outline" colorPalette="gray"
            disabled={disabled || of >= MAX_PEOPLE} onClick={() => onChange(of + 1)}>
            <LuPlus />
          </Button>
        </HStack>
      </HStack>
      <VStack align="stretch" gap={1} maxH="30dvh" overflowY="auto">
        {Array.from({ length: of }, (_, i) => i + 1).map((part) => {
          const paid = payments.find((p) => !p.voided && p.split?.of === of && p.split.part === part);
          const now = part === current;
          return (
            <HStack key={part} justify="space-between" px={3} py={2} minH="44px" borderRadius="lg"
              borderWidth="1px" borderColor={now ? 'colorPalette.solid' : 'border'} bg={paid ? 'bg.muted' : undefined}>
              <Text fontWeight="600">Parte {part} de {of}</Text>
              <Text fontSize="sm" color={paid ? 'fg.muted' : undefined}>
                {paid ? `Pagada · ${paid.methodName} · ${money(paid.amount, currency)}`
                  : now ? (currentAmount !== null ? `Se cobra ahora · ${money(currentAmount, currency)}` : 'Se cobra ahora')
                    : 'Por cobrar'}
              </Text>
            </HStack>
          );
        })}
      </VStack>
      {paidBefore && (
        <Box px={3} py={2} borderRadius="md" bg="bg.muted">
          <Text fontSize="sm" color="fg.muted">
            Ya hay pagos en este pedido; esto reparte solo lo que falta ({money(outstanding, currency)}).
          </Text>
        </Box>
      )}
    </VStack>
  );
}
