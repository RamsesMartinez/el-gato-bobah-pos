import { Box, HStack, Input, Text } from '@chakra-ui/react';
import { fondoExcedeLoContado } from './fondoQueSeDeja';
import { money } from '../../utils/format';

// FondoQueSeDeja: cuánto se queda en el cajón para el siguiente turno; el resto se retira
// (decisión del dueño del 2026-10-10). La apertura siguiente se compara contra esta cifra.
export function FondoQueSeDeja({ value, onChange, contado = null }: {
  value: string; onChange: (v: string) => void; contado?: number | null;
}) {
  const excede = fondoExcedeLoContado(value, contado);
  return (
    <Box borderWidth="1px" borderRadius="lg" p={3} borderColor={excede ? 'red.400' : undefined}>
      <HStack justify="space-between" gap={3}>
        <Text fontWeight="700">Se queda de fondo para el siguiente turno</Text>
        <Input aria-label="Se queda de fondo" inputMode="decimal" maxW="180px" minH="48px" placeholder="$"
          value={value} onChange={(e) => onChange(e.target.value)} />
      </HStack>
      {excede && contado !== null && (
        <Text mt={2} fontSize="sm" color="red.600" _dark={{ color: 'red.300' }}>
          Es más de lo contado en el cajón ({money(contado)}).
        </Text>
      )}
    </Box>
  );
}
