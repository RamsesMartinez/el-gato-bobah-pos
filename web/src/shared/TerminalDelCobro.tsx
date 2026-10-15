import { Button, HStack, Text, Wrap } from '@chakra-ui/react';
import { Picker } from '../components/Picker';
import type { CardTerminal } from '../api/backoffice';

import { terminalEfectiva } from './terminalEfectiva';

// Hasta cuántas terminales caben como botones en la hoja de cobro sin empujar el botón de cobrar.
const BOTONES_MAX = 4;

// TerminalDelCobro: con tarjeta se registra la terminal (spec 032, punto 8). Con varias se elige en
// cada cobro y nada llega marcado (spec 036): con pocas van a la vista, para que sea un toque y no
// dos; con una sola no hay nada que elegir.
export function TerminalDelCobro({ terminales, elegida, onChange }: {
  terminales: CardTerminal[];
  elegida: number | null;
  onChange: (id: number) => void;
}) {
  const efectiva = terminalEfectiva(terminales, elegida);
  const activas = terminales.filter((t) => !t.archived);
  return (
    <HStack gap={3} align="center">
      <Text fontSize="sm" fontWeight="600" w="84px" flexShrink={0}>Terminal</Text>
      <HStack flex="1" minW={0}>
        {activas.length === 1 ? (
          <Text fontSize="sm" fontWeight="700">{activas[0].name}</Text>
        ) : activas.length <= BOTONES_MAX ? (
          <Wrap gap={2}>
            {activas.map((t) => (
              <Button key={t.id} minH="52px" px={5} fontWeight="700"
                variant={efectiva === t.id ? 'solid' : 'outline'}
                aria-pressed={efectiva === t.id}
                onClick={() => onChange(t.id)}>
                {t.name}
              </Button>
            ))}
          </Wrap>
        ) : (
          <Picker value={efectiva === null ? '' : String(efectiva)}
            options={activas.map((t) => ({ value: String(t.id), label: t.name }))}
            onChange={(v) => onChange(Number(v))} placeholder="Elige la terminal" title="Terminal" />
        )}
      </HStack>
    </HStack>
  );
}
