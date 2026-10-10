import { Box, HStack, Input, Text, VStack } from '@chakra-ui/react';

import { type TerminalPorContar } from './terminalesPorContar';

// ConteoDeTerminales: con arqueo por terminal se escribe el total que imprime el corte de cada
// terminal que cobró en el turno (spec 032, punto 9). La diferencia sale en el corte al cerrar.
export function ConteoDeTerminales({ terminales, valores, onChange }: {
  terminales: TerminalPorContar[];
  valores: Record<number, string>;
  onChange: (v: Record<number, string>) => void;
}) {
  if (terminales.length === 0) return null;
  return (
    <Box borderWidth="1px" borderRadius="lg" p={3}>
      <Text fontWeight="700" mb={2}>Tarjeta: total del corte de cada terminal</Text>
      <VStack align="stretch" gap={2}>
        {terminales.map((t) => (
          <HStack key={t.terminalId} justify="space-between">
            <Text fontWeight="600">{t.name}</Text>
            <Input aria-label={`Total del corte de ${t.name}`} inputMode="decimal" maxW="180px" minH="48px" placeholder="$"
              value={valores[t.terminalId] ?? ''}
              onChange={(e) => onChange({ ...valores, [t.terminalId]: e.target.value })} />
          </HStack>
        ))}
      </VStack>
    </Box>
  );
}
