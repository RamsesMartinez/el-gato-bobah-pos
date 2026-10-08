import { Button, HStack, SimpleGrid, Text, VStack } from '@chakra-ui/react';
import { LuDelete } from 'react-icons/lu';
import type { Currency } from '../../types/pos';
import { money } from '../../utils/format';
import { round2 } from '../../domain/cobro';
import { KEYS, typeKey } from './split';

const QUICK = [100, 200, 300];

interface Props {
  value: string;
  outstanding: number;
  currency: Currency;
  disabled?: boolean;
  onChange: (value: string) => void;
}

// ByAmount es «¿Cuánto paga esta persona?» con un teclado propio: el del sistema se come la mitad de
// los 600 px de alto y tapa justo la cifra que decide si el botón se enciende.
export function ByAmount({ value, outstanding, currency, disabled, onChange }: Props) {
  const typed = Number(value || 0);
  const left = Math.max(0, round2(outstanding - typed));
  return (
    <VStack align="stretch" gap={2}>
      <HStack justify="space-between" align="end">
        <VStack align="start" gap={0}>
          <Text fontSize="sm" fontWeight="600">¿Cuánto paga esta persona?</Text>
          <Text fontSize="3xl" fontWeight="800" aria-label="Monto">{value ? money(value, currency) : '$0'}</Text>
        </VStack>
        <VStack align="end" gap={0}>
          <Text fontSize="xs" color="fg.muted">Después de este pago faltan</Text>
          <Text fontWeight="700">{money(String(left), currency)}</Text>
        </VStack>
      </HStack>
      <HStack gap={2} flexWrap="wrap">
        {QUICK.filter((q) => q < outstanding).map((q) => (
          <Button key={q} minH="44px" variant="outline" colorPalette="gray" disabled={disabled}
            onClick={() => onChange(String(q))}>{money(String(q), currency)}</Button>
        ))}
        <Button minH="44px" variant="outline" colorPalette="gray" disabled={disabled}
          onClick={() => onChange(String(outstanding))}>
          Lo que falta · {money(String(outstanding), currency)}
        </Button>
      </HStack>
      <SimpleGrid columns={3} gap={2} maxW="22rem">
        {KEYS.map((k) => (
          <Button key={k} minH="52px" variant="outline" colorPalette="gray" fontSize="xl" disabled={disabled}
            aria-label={k === 'back' ? 'Borrar' : k === '.' ? 'Punto' : k}
            onClick={() => onChange(typeKey(value, k))}>
            {k === 'back' ? <LuDelete /> : k}
          </Button>
        ))}
      </SimpleGrid>
    </VStack>
  );
}
