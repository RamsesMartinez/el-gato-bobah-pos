import {
  Box, Flex, HStack, VStack, Text, Button, IconButton, Separator, Center,
} from '@chakra-ui/react';
import { LuTrash2, LuStickyNote } from 'react-icons/lu';
import { useTicketStore, useActiveTicket, lineTotal, ticketTotal } from '../../stores/ticket';
import type { TicketLine } from '../../types/pos';
import { money } from '../../utils/format';

interface Props {
  onCheckout: () => void;
  onEditLine: (line: TicketLine) => void;
}

export function Ticket({ onCheckout, onEditLine }: Props) {
  const { lines, customerName } = useActiveTicket();
  const inc = useTicketStore((s) => s.incrementLine);
  const dec = useTicketStore((s) => s.decrementLine);
  const remove = useTicketStore((s) => s.removeLine);
  const clear = useTicketStore((s) => s.clearActive);
  const total = ticketTotal(lines);

  return (
    <Flex direction="column" h="100%" bg="bg.panel">
      <HStack justify="space-between" p={4} pb={2}>
        <Text fontWeight="700" fontSize="lg">
          Pedido {customerName && <Text as="span" color="fg.muted" fontWeight="500">· {customerName}</Text>}
        </Text>
        {lines.length > 0 && (
          <Button size="xs" variant="ghost" colorPalette="red" onClick={() => { if (confirm('¿Vaciar pedido?')) clear(); }}>
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
                      .map((m) => {
                        const label = m.portion ? `½${m.portion} ${m.name}` : m.name;
                        return m.qty > 1 ? `${label} ×${m.qty}` : label;
                      })
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
