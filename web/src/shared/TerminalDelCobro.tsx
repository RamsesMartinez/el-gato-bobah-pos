import { HStack, Text } from '@chakra-ui/react';
import { Picker } from '../components/Picker';
import type { CardTerminal } from '../api/backoffice';

import { terminalEfectiva } from './terminalEfectiva';

// TerminalDelCobro: con tarjeta se registra la terminal (spec 032, punto 8). Llega puesta; cambiarla
// es un toque en la misma hoja.
export function TerminalDelCobro({ terminales, porOmision, elegida, onChange }: {
  terminales: CardTerminal[];
  porOmision: number | null;
  elegida: number | null;
  onChange: (id: number) => void;
}) {
  const efectiva = terminalEfectiva(terminales, porOmision, elegida);
  const opciones = terminales.filter((t) => !t.archived).map((t) => ({ value: String(t.id), label: t.name }));
  return (
    <HStack gap={3} align="center">
      <Text fontSize="sm" fontWeight="600" w="84px" flexShrink={0}>Terminal</Text>
      <HStack flex="1" minW={0}>
        <Picker value={efectiva === null ? '' : String(efectiva)} options={opciones}
          onChange={(v) => onChange(Number(v))} placeholder="Elige la terminal" title="Terminal" />
      </HStack>
    </HStack>
  );
}
