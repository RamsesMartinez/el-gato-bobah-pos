import { HStack, Button, Box, Text } from '@chakra-ui/react';
import { LuPlus, LuX } from 'react-icons/lu';
import { useTicketStore, ticketCount } from '../../stores/ticket';
import { RADIUS, BORDER_W } from '../../theme/ui';

// Barra de cuentas abiertas: crear, cambiar y cerrar pedidos en curso.
export function TicketTabs() {
  const tabs = useTicketStore((s) => s.tabs);
  const activeId = useTicketStore((s) => s.activeId);
  const switchTab = useTicketStore((s) => s.switchTab);
  const closeTab = useTicketStore((s) => s.closeTab);
  const newTab = useTicketStore((s) => s.newTab);

  const close = (id: string, hasLines: boolean) => {
    if (hasLines && !confirm('¿Cerrar esta cuenta y descartar sus artículos?')) return;
    closeTab(id);
  };

  return (
    <HStack gap={2} overflowX="auto" py={1} css={{ scrollbarWidth: 'none', '&::-webkit-scrollbar': { display: 'none' } }}>
      {tabs.map((t) => {
        const active = t.id === activeId;
        const count = ticketCount(t.lines);
        return (
          <HStack
            key={t.id}
            as="button"
            onClick={() => switchTab(t.id)}
            flexShrink={0}
            gap={1}
            h="44px"
            pl={3}
            pr={1.5}
            borderRadius={RADIUS}
            borderWidth={BORDER_W}
            borderColor={active ? 'colorPalette.600' : 'border'}
            bg={active ? 'colorPalette.600' : 'bg.panel'}
            color={active ? 'white' : 'fg'}
          >
            <Text fontWeight="600" fontSize="sm" whiteSpace="nowrap">
              {t.customerName || `Cuenta ${t.num}`}{count > 0 ? ` · ${count}` : ''}
            </Text>
            <Box
              as="span"
              p={1}
              borderRadius="full"
              _hover={{ bg: active ? 'whiteAlpha.300' : 'bg.muted' }}
              onClick={(e) => { e.stopPropagation(); close(t.id, count > 0); }}
            >
              <LuX size={14} />
            </Box>
          </HStack>
        );
      })}
      <Button flexShrink={0} size="sm" h="44px" variant="outline" onClick={newTab}>
        <LuPlus /> Cuenta
      </Button>
    </HStack>
  );
}
