import { HStack, Input, Text } from '@chakra-ui/react';

// FondoQueSeDeja: cuánto se queda en el cajón para el siguiente turno; el resto se retira
// (decisión del dueño del 2026-10-10). La apertura siguiente se compara contra esta cifra.
export function FondoQueSeDeja({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <HStack borderWidth="1px" borderRadius="lg" p={3} justify="space-between" gap={3}>
      <Text fontWeight="700">Se queda de fondo para el siguiente turno</Text>
      <Input aria-label="Se queda de fondo" inputMode="decimal" maxW="180px" minH="48px" placeholder="$"
        value={value} onChange={(e) => onChange(e.target.value)} />
    </HStack>
  );
}
