import {
  Box, Flex, HStack, VStack, Text, Button, IconButton, Separator, Center, Input,
} from '@chakra-ui/react';
import { LuTrash2, LuStickyNote, LuPanelRightClose, LuStore, LuBike } from 'react-icons/lu';
import { useTicketStore, useActiveTicket, lineTotal, ticketTotal } from '../../stores/ticket';
import type { TicketLine } from '../../types/pos';
import type { SwipeHandlers } from '../../hooks/useSwipeDownToClose';
import { money } from '../../utils/format';

// Los dos destinos de una cuenta. Se ofrecen aquí y no dentro de la hoja de cobro porque son
// decisiones sobre el PEDIDO, no sobre el dinero: mandar a cocina no cobra nada, y tenerlo junto al
// método de pago hacía que la pantalla pidiera propina para algo que podía no cobrarse.
interface Props {
  onCheckout: () => void;
  // Manda a cocina sin cobrar. Queda por cobrar y el tablero lo marca.
  onEnviar: () => void;
  enviando?: boolean;
  onEditLine: (line: TicketLine) => void;
  onHide?: () => void; // ocultar el panel lateral (solo modo ancho)
  // solo cuando Ticket vive dentro del bottom sheet (modo angosto): arrastrar el header
  // hacia abajo también lo cierra, además del botón "Ocultar pedido".
  swipeHandlers?: SwipeHandlers;
}

// para_llevar salió del selector: no cambiaba nada y ningún reporte agrupaba por él.
const TIPOS = [
  { v: 'mostrador' as const, label: 'Mostrador', icon: LuStore },
  { v: 'domicilio' as const, label: 'Domicilio', icon: LuBike },
];

export function Ticket({ onCheckout, onEnviar, enviando, onEditLine, onHide, swipeHandlers }: Props) {
  const { lines, customerName, folioName, serviceType, platformId } = useActiveTicket();
  const setServiceType = useTicketStore((s) => s.setServiceType);
  const setCustomerName = useTicketStore((s) => s.setCustomerName);
  const inc = useTicketStore((s) => s.incrementLine);
  const dec = useTicketStore((s) => s.decrementLine);
  const remove = useTicketStore((s) => s.removeLine);
  const clear = useTicketStore((s) => s.clearActive);
  const total = ticketTotal(lines);

  return (
    <Flex direction="column" h="100%" bg="bg.panel">
      <HStack justify="space-between" p={3} pb={2} gap={2}
        style={swipeHandlers ? { touchAction: 'none' } : undefined} {...swipeHandlers}>
        {/* collapse a la IZQUIERDA, lejos de "Vaciar" (destructivo) para evitar toques accidentales en 7" */}
        <HStack gap={2} minW={0} flex="1">
          {onHide && (
            <IconButton size="lg" minW="48px" minH="48px" variant="ghost" colorPalette="gray"
              aria-label="Ocultar pedido" onClick={onHide}>
              <LuPanelRightClose />
            </IconButton>
          )}
          <Text fontWeight="700" fontSize="lg" truncate>
            Pedido{' '}
            {/* Tenue y detrás de "Pedido": es con lo que se va a cantar en cocina, y se ve desde
                aquí para poder decírselo al cliente al tomarle el pedido. */}
            <Text as="span" color="fg.subtle" fontWeight="500">{folioName}</Text>
            {customerName && <Text as="span" color="fg.muted" fontWeight="500"> · {customerName}</Text>}
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
      <Box p={3}>
        <Flex justify="space-between" align="center" mb={2}>
          <Text fontSize="lg" fontWeight="600">Total</Text>
          <Text fontSize="2xl" fontWeight="800">{money(total)}</Text>
        </Flex>

        {/* Tipo y cliente son del PEDIDO, así que se capturan mientras se toma, no al cobrar. Un
            pedido de plataforma ya es a domicilio por definición y no admite otro tipo. */}
        {platformId === null && (
          <HStack gap={2} mb={2}>
            <HStack gap={1} flexShrink={0}>
              {TIPOS.map((t) => (
                <Button key={t.v} size="sm" minH="44px" px={2.5}
                  variant={serviceType === t.v ? 'solid' : 'outline'}
                  colorPalette={serviceType === t.v ? undefined : 'gray'}
                  onClick={() => setServiceType(t.v)}>
                  <t.icon /> {t.label}
                </Button>
              ))}
            </HStack>
            <Input flex="1" minW={0} minH="44px" placeholder="Cliente"
              value={customerName} onChange={(e) => setCustomerName(e.target.value)} />
          </HStack>
        )}

        <HStack gap={2}>
          {/* Enviar es secundario y COBRAR domina: cobrar es lo que pasa en casi toda venta, y dos
              botones con el mismo peso invitan al toque equivocado — que aquí significa creer que
              se cobró algo que no se cobró. */}
          <Button flex="1" size="lg" h="56px" variant="outline" colorPalette="gray"
            disabled={lines.length === 0} loading={enviando} onClick={onEnviar}>
            Enviar a cocina
          </Button>
          <Button flex="1.3" size="lg" h="56px" colorPalette="green" fontWeight="800"
            disabled={lines.length === 0} onClick={onCheckout}>
            COBRAR
          </Button>
        </HStack>
      </Box>
    </Flex>
  );
}
