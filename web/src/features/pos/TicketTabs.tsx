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
            pr={active ? 1 : 3}
            borderRadius={RADIUS}
            borderWidth={BORDER_W}
            borderColor={active ? 'colorPalette.600' : 'border'}
            bg={active ? 'colorPalette.600' : 'bg.panel'}
            color={active ? 'white' : 'fg'}
          >
            <Text fontWeight="600" fontSize="sm" whiteSpace="nowrap">
              {t.customerName || `Cuenta ${t.num}`}{count > 0 ? ` · ${count}` : ''}
            </Text>
            {/* la ✕ solo en la cuenta activa → no se cierra por error una de fondo al cambiar.
                target amplio (32px, para 7") + confirmación si tiene artículos (fn close). */}
            {active && (
              <Box
                as="span"
                role="button"
                aria-label="Cerrar cuenta"
                display="flex"
                alignItems="center"
                justifyContent="center"
                minW="32px"
                minH="32px"
                borderRadius="full"
                _hover={{ bg: 'whiteAlpha.300' }}
                _active={{ bg: 'whiteAlpha.400' }}
                onClick={(e) => { e.stopPropagation(); close(t.id, count > 0); }}
              >
                <LuX size={16} />
              </Box>
            )}
          </HStack>
        );
      })}
      <Button flexShrink={0} size="sm" h="44px" variant="outline" onClick={newTab}>
        <LuPlus /> Cuenta
      </Button>
    </HStack>
  );
}
