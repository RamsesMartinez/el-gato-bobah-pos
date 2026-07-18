import {
  Box, Flex, HStack, VStack, Text, Button, IconButton, Separator, Center,
} from '@chakra-ui/react';
import { LuTrash2, LuStickyNote, LuPanelRightClose } from 'react-icons/lu';
import { useTicketStore, useActiveTicket, lineTotal, ticketTotal } from '../../stores/ticket';
import type { TicketLine } from '../../types/pos';
import { money } from '../../utils/format';

interface Props {
  onCheckout: () => void;
  onEditLine: (line: TicketLine) => void;
  onHide?: () => void; // ocultar el panel lateral (solo modo ancho)
}

export function Ticket({ onCheckout, onEditLine, onHide }: Props) {
  const { lines, customerName } = useActiveTicket();
  const inc = useTicketStore((s) => s.incrementLine);
  const dec = useTicketStore((s) => s.decrementLine);
  const remove = useTicketStore((s) => s.removeLine);
  const clear = useTicketStore((s) => s.clearActive);
  const total = ticketTotal(lines);

  return (
    <Flex direction="column" h="100%" bg="bg.panel">
      <HStack justify="space-between" p={3} pb={2} gap={2}>
        {/* collapse a la IZQUIERDA, lejos de "Vaciar" (destructivo) para evitar toques accidentales en 7" */}
        <HStack gap={2} minW={0} flex="1">
          {onHide && (
            <IconButton size="sm" minW="40px" minH="40px" variant="ghost" colorPalette="gray"
              aria-label="Ocultar pedido" onClick={onHide}>
              <LuPanelRightClose />
            </IconButton>
          )}
          <Text fontWeight="700" fontSize="lg" truncate>
            Pedido {customerName && <Text as="span" color="fg.muted" fontWeight="500">· {customerName}</Text>}
          </Text>
        </HStack>
        {lines.length > 0 && (
          <Button size="sm" minH="40px" px={3} variant="ghost" colorPalette="red"
            onClick={() => { if (confirm('¿Vaciar pedido?')) clear(); }}>
            Vaciar
          </Button>
        )}
      </HStack>
      <Separator />

      <VStack align="stretch" gap={0} flex="1" overflowY="auto" px={2} py={2}>
        {lines.length === 0 && (
          <Center h="100%" px={6}>
            <Text color="fg.subtle" textAlign="center">Toca un producto para agregarlo</Text>
          </Center>
        )}
        {lines.map((l) => (
          <Box key={l.lineId} py={2} px={2} borderBottomWidth="1px" borderColor="border.muted">
            <Flex justify="space-between" gap={2}>
              <Box flex="1" onClick={() => onEditLine(l)} cursor="pointer">
                <Text fontWeight="600" fontSize="sm">{l.name}</Text>
                {l.modifiers.length > 0 && (
                  <Text fontSize="xs" color="fg.muted" lineClamp={2}>
                    {l.modifiers
                      .map((m) => (m.qty > 1 ? `${m.name} ×${m.qty}` : m.name))
                      .join(' · ')}
                  </Text>
                )}
                {l.notes && (
                  <HStack gap={1} color="orange.500">
                    <LuStickyNote size={12} />
                    <Text fontSize="xs">{l.notes}</Text>
                  </HStack>
                )}
              </Box>
              <Text fontWeight="600" fontSize="sm" whiteSpace="nowrap">{money(lineTotal(l))}</Text>
            </Flex>
            <HStack mt={1} gap={1}>
              <IconButton aria-label="Quitar" size="xs" variant="ghost" colorPalette="red" onClick={() => remove(l.lineId)}><LuTrash2 /></IconButton>
              <Button size="xs" onClick={() => dec(l.lineId)}>−</Button>
              <Text minW="24px" textAlign="center" fontSize="sm">{l.qty}</Text>
              <Button size="xs" onClick={() => inc(l.lineId)}>+</Button>
            </HStack>
          </Box>
        ))}
      </VStack>

      <Separator />
      <Box p={4}>
        <Flex justify="space-between" mb={3}>
          <Text fontSize="lg" fontWeight="600">Total</Text>
          <Text fontSize="2xl" fontWeight="800">{money(total)}</Text>
        </Flex>
        <Button w="100%" size="lg" h="56px" disabled={lines.length === 0} onClick={onCheckout}>
          COBRAR
        </Button>
      </Box>
    </Flex>
  );
}
