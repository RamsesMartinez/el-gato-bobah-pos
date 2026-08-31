import { HStack, VStack, Button, Box, Text } from '@chakra-ui/react';
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
            {/* El animal va tenue y debajo: identifica al pedido en cocina, pero quien mira las
                pestañas está eligiendo en cuál capturar, y para eso sirve el número de cuenta o el
                nombre del cliente. Se ve desde aquí para que el operador pueda decírselo al cliente
                al tomarle el pedido, no hasta que imprime el ticket. */}
            <VStack gap={0} align="start">
              <Text fontWeight="600" fontSize="sm" whiteSpace="nowrap" lineHeight="1.15">
                {t.customerName || `Cuenta ${t.num}`}{count > 0 ? ` · ${count}` : ''}
              </Text>
              {/* El espacio duro sostiene el renglón mientras la lista de animales llega del
                  servidor: sin él la pestaña se recentra sola al aparecer el nombre. */}
              <Text fontSize="2xs" whiteSpace="nowrap" lineHeight="1.15"
                color={active ? 'whiteAlpha.700' : 'fg.subtle'}>
                {t.folioName || ' '}
              </Text>
            </VStack>
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
