import {
  Box, Flex, HStack, VStack, Text, Button, IconButton, Separator, Center, Input,
} from '@chakra-ui/react';
import { LuTrash2, LuStickyNote, LuPanelRightClose, LuStore, LuBike } from 'react-icons/lu';
import { useTicketStore, useActiveTicket, lineTotal, ticketTotal } from '../../stores/ticket';
import { envioDeLaCuenta } from '../../domain/envio';
import type { TicketLine } from '../../types/pos';
import type { SwipeHandlers } from '../../hooks/useSwipeDownToClose';
import { money } from '../../utils/format';

// Los dos destinos de una cuenta. Se ofrecen aquí y no dentro de la hoja de cobro porque son
// decisiones sobre el PEDIDO, no sobre el dinero: mandar a cocina no cobra nada, y tenerlo junto al
// método de pago hacía que la pantalla pidiera propina para algo que podía no cobrarse.
interface Props {
  onCheckout: () => void;
  // Costo de envío de ESTE pedido, y el default del negocio. Viven en el panel y no en una pantalla
  // de cobro porque son atributos del pedido que se está armando: el operador los decide mientras
  // toma la orden, no cuando cuenta el dinero. Vacío = el default del negocio.
  envioPorDefecto: number;
  // Renglones que el servidor va a rechazar porque el producto se inactivó mientras estaba en el
  // carrito. Se avisa AQUÍ, mientras se puede quitar, y no al cobrar con el cliente enfrente.
  noDisponibles: TicketLine[];
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

export function Ticket({
  onCheckout, onEnviar, enviando, onEditLine, onHide, swipeHandlers,
  envioPorDefecto, noDisponibles,
}: Props) {
  const { lines, customerName, folioName, serviceType, platformId, envio } = useActiveTicket();
  const setServiceType = useTicketStore((s) => s.setServiceType);
  const setCustomerName = useTicketStore((s) => s.setCustomerName);
  const setEnvio = useTicketStore((s) => s.setEnvio);
  const inc = useTicketStore((s) => s.incrementLine);
  const dec = useTicketStore((s) => s.decrementLine);
  const remove = useTicketStore((s) => s.removeLine);
  const clear = useTicketStore((s) => s.clearActive);
  const total = ticketTotal(lines);
  // La MISMA función que usan la píldora y la barra angosta. Cada superficie la llama con la misma
  // cuenta; lo que no puede haber es dos implementaciones, que es como el panel acabó apagando sus
  // botones por un envío que la píldora cobraba al default.
  const envioDelPedido = envioDeLaCuenta({ serviceType, platformId }, envio, envioPorDefecto);
  const llevaEnvio = envioDelPedido.aplica;
  const envioMalEscrito = envioDelPedido.malEscrito;

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
          <Box key={l.lineId} py={1.5} px={2} borderBottomWidth="1px" borderColor="border.muted">
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
            {/* Los controles del renglón medían ~24 px, por debajo del piso de 44 que la
                constitución fija: por ahí el dedo toca dos veces y la segunda cae en otra cosa —
                y aquí "otra cosa" era la papelera, que estaba pegada al menos.

                La papelera se va al EXTREMO OPUESTO. Separar la acción destructiva de la frecuente
                es requisito funcional, no gusto: quitar un renglón por accidente con el cliente
                enfrente obliga a volver a buscarlo en el menú.

                Cuesta ~14 px de alto por renglón (los 20 que suben los controles, menos los 6 que
                se recuperan apretando el relleno). En el panel de 600 px eso es un renglón menos a
                la vista, y se paga: un control que no se puede tocar no sirve de nada. */}
            <HStack mt={0.5} gap={1} justify="space-between">
              <HStack gap={1}>
                <Button size="sm" minW="44px" minH="44px" onClick={() => dec(l.lineId)}>−</Button>
                <Text minW="32px" textAlign="center" fontSize="sm" fontWeight="600">{l.qty}</Text>
                <Button size="sm" minW="44px" minH="44px" onClick={() => inc(l.lineId)}>+</Button>
              </HStack>
              <IconButton aria-label="Quitar" size="sm" minW="44px" minH="44px" variant="ghost"
                colorPalette="red" onClick={() => remove(l.lineId)}><LuTrash2 /></IconButton>
            </HStack>
          </Box>
        ))}
      </VStack>

      <Separator />
      <Box p={3}>
        {/* Lo que el servidor ya no acepta. Se dice mientras la cuenta se está armando y se puede
            quitar de un toque; enterarse al cobrar deja al operador resolviéndolo con el cliente
            enfrente. */}
        {noDisponibles.length > 0 && (
          <Box colorPalette="orange" borderWidth="1px" borderColor="colorPalette.emphasized"
            bg="colorPalette.subtle" borderRadius="lg" p={2} mb={2}>
            <Text fontWeight="700" fontSize="sm" color="colorPalette.fg">
              Ya no están en el menú
            </Text>
            <Text fontSize="xs" color="fg.muted" mb={2}>
              {noDisponibles.map((l) => l.name).join(', ')}
            </Text>
            <Button size="sm" minH="44px" variant="outline" colorPalette="orange"
              onClick={() => noDisponibles.forEach((l) => remove(l.lineId))}>
              Quitar del pedido
            </Button>
          </Box>
        )}

        <Flex justify="space-between" align="center" mb={2}>
          <Text fontSize="lg" fontWeight="600">Total</Text>
          <Text fontSize="2xl" fontWeight="800">{money(total + envioDelPedido.monto)}</Text>
        </Flex>

        {/* El envío solo cuando el pedido lo cobra el negocio. Con plataforma no aparece: lo cobra
            ella, y la regla la contesta `cobraEnvio` en vez de deducirla aquí — deducirla fue como
            la pantalla llegó a ofrecer un envío que el servidor no cobra. */}
        {llevaEnvio && (
          <HStack gap={2} mb={2}>
            <Text fontSize="sm" color="fg.muted" flexShrink={0}>Envío</Text>
            <Input flex="1" minH="44px" inputMode="decimal" aria-label="Costo de envío"
              placeholder={money(envioPorDefecto)}
              value={envio} onChange={(e) => setEnvio(e.target.value)} />
            {envioMalEscrito && (
              <Text fontSize="xs" color="red.fg">Solo números</Text>
            )}
          </HStack>
        )}

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
            disabled={lines.length === 0 || envioMalEscrito} loading={enviando} onClick={onEnviar}>
            Enviar a cocina
          </Button>
          {/* COBRAR también MANDA el pedido a cocina, y decirlo importa: desde que confirmar es
              obligatorio antes de cobrar, este botón dejó de ser reversible. Quien lo toca y luego
              cierra la hoja de cobro creyendo que canceló, ve el carrito vacío y vuelve a capturar
              el pedido — y cocina prepara dos veces lo mismo. */}
          {/* `loading` también aquí: es el botón que más se toca y era el único de los dos sin
              estado ocupado. En red lenta el operador toca dos veces y salen dos POST /orders
              concurrentes con el mismo clientUuid; la idempotencia de Create es check-then-insert
              sin lock, así que la segunda choca contra el índice único y sale un 500 sobre un
              pedido que SÍ se creó — con la cuenta ya cerrada, recapturar manda dos veces a cocina. */}
          <Button flex="1.3" size="lg" h="56px" colorPalette="green" fontWeight="800"
            aria-describedby="cobrar-manda-a-cocina" loading={enviando}
            disabled={lines.length === 0 || envioMalEscrito} onClick={onCheckout}>
            COBRAR
          </Button>
        </HStack>
        <Text id="cobrar-manda-a-cocina" fontSize="2xs" color="fg.muted" textAlign="right" mt={1}>
          Cobrar también manda el pedido a cocina
        </Text>
      </Box>
    </Flex>
  );
}
