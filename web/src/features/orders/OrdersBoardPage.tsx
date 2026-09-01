import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Box, SimpleGrid, Text, Badge, HStack, VStack, Center, Spinner, Flex, Button, IconButton,
} from '@chakra-ui/react';
import { MenuRoot, MenuTrigger, MenuContent, MenuItem } from '../../components/ui/menu';
import { LuStore, LuBike, LuEllipsisVertical, LuMinus, LuPlus, LuCheck, LuShoppingBag } from 'react-icons/lu';
import type { IconType } from 'react-icons';
import { toaster } from '../../components/ui/toaster';
import { posApi } from '../../api/pos';
import type { BoardLine, BoardOrder } from '../../types/pos';
import { resumenPorCobrar } from './porCobrar';
import { entregados, faltante, pendientes } from './entrega';
import { money } from '../../utils/format';
import { useOrderEvents } from '../../hooks/useOrderEvents';
import { ReprintTicket } from '../tickets/ReprintTicket';
import { CobrarSheet } from './CobrarSheet';
import { useSessionStore } from '../../stores/session';

// para_llevar ya no se ofrece al cobrar, pero hay pedidos históricos con ese tipo y sin su etiqueta
// la tarjeta los mostraría como "para_llevar", con guion bajo.
const SERVICE_META: Record<string, { label: string; icon: IconType }> = {
  mostrador: { label: 'Mostrador', icon: LuStore },
  para_llevar: { label: 'Llevar', icon: LuShoppingBag },
  domicilio: { label: 'Domicilio', icon: LuBike },
};

// Alto mínimo de todo lo que se toca. Por debajo el dedo falla, y aquí fallar significa dar por
// entregado o por cobrado lo que no fue.
const TAP = '44px';

// Cuántas entregadas se listan. El resto vive en Ventas, que es la pantalla del histórico; aquí
// estorbarían lo que falta por atender.
const ENTREGADAS_VISIBLES = 5;

const CANCEL_REASONS = ['Cliente canceló', 'Error de captura', 'Sin insumos', 'Otro'];
const REFUND_REASONS = ['Producto mal', 'Se cayó / dañó', 'Queja del cliente', 'Cobro erróneo', 'Otro'];

export function OrdersBoardPage() {
  const live = useOrderEvents();
  const [ticketOrderID, setTicketOrderID] = useState<number | null>(null);
  const [cobrando, setCobrando] = useState<BoardOrder | null>(null);
  const qc = useQueryClient();
  // Reembolsar = salida de dinero → solo admin/gerente ven las entregadas y la acción. El backend
  // igual aplica el 403; esto es UX (no mostrar lo que no pueden usar).
  const role = useSessionStore((s) => s.user?.role);
  const canRefund = role === 'admin' || role === 'gerente';

  // Cobrar es del punto de venta. Este tablero prepara y entrega, y solo recupera el botón donde
  // el negocio lo enciende a propósito: en un local donde cocina y mostrador son la misma persona
  // en la misma máquina, mandarla a otra pantalla por un pedido que tiene enfrente no compra nada.
  const { data: settings } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });
  const puedeCobrar = settings?.kitchenCanCharge === true;

  const { data, isLoading } = useQuery({
    queryKey: ['orders', 'active'],
    queryFn: posApi.activeOrders,
    refetchInterval: 10_000, // respaldo si SSE se cae
  });
  const { data: deliveredData } = useQuery({
    queryKey: ['orders', 'delivered'],
    queryFn: posApi.deliveredOrders,
    enabled: canRefund,
    refetchInterval: 15_000, // SSE solo invalida 'active'; refrescamos entregadas aparte
  });

  const invalidateAll = () => {
    qc.invalidateQueries({ queryKey: ['orders', 'active'] });
    qc.invalidateQueries({ queryKey: ['orders', 'delivered'] });
  };
  const conError = (titulo: string) => (e: unknown) =>
    toaster.create({ title: titulo, description: String(e), type: 'error' });

  const entregarLinea = useMutation({
    mutationFn: ({ id, lineId, qty }: { id: number; lineId: number; qty: number }) =>
      posApi.deliverLine(id, lineId, qty),
    onSuccess: invalidateAll,
    onError: conError('No se pudo entregar'),
  });
  const entregarTodo = useMutation({
    mutationFn: (id: number) => posApi.deliverOrder(id),
    onSuccess: invalidateAll,
    onError: conError('No se pudo entregar'),
  });
  const cancelMut = useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) => posApi.cancelOrder(id, reason),
    onSuccess: invalidateAll,
    // Un pedido del que ya salió comida no se cancela: reponer el stock de lo que el cliente se
    // llevó le inventaría existencias al almacén. El servidor lo rechaza y aquí se dice por qué.
    onError: conError('No se pudo cancelar'),
  });
  const refundMut = useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) => posApi.refundOrder(id, reason),
    onSuccess: () => { invalidateAll(); toaster.create({ title: 'Reembolso registrado', type: 'success' }); },
    onError: conError('Error'),
  });

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;
  const orders = data?.items ?? [];
  const preparando = orders.filter((o) => o.status === 'abierta');
  const listos = orders.filter((o) => o.status === 'lista');
  const entregadas = deliveredData?.items ?? [];
  // Cuenta las entregadas también: es donde un pendiente deja de tener remedio, porque el cliente
  // ya se fue con la comida.
  const pendiente = resumenPorCobrar([...orders, ...entregadas]);

  const cancel = (o: BoardOrder) => {
    const reason = window.prompt(`Motivo de cancelación:\n(${CANCEL_REASONS.join(', ')})`, CANCEL_REASONS[0]);
    if (reason) cancelMut.mutate({ id: o.id, reason });
  };
  const refund = (o: BoardOrder) => {
    const reason = window.prompt(`Motivo del reembolso:\n(${REFUND_REASONS.join(', ')})`, REFUND_REASONS[0]);
    if (reason?.trim()) refundMut.mutate({ id: o.id, reason: reason.trim() });
  };

  const acciones: Acciones = {
    puedeCobrar,
    entregarLinea: (id, lineId, qty) => entregarLinea.mutate({ id, lineId, qty }),
    entregarTodo: (o) => entregarTodo.mutate(o.id),
    cobrar: setCobrando,
    ticket: (o) => setTicketOrderID(o.id),
    cancelar: cancel,
  };

  return (
    // p={3} y no p={4}: cada píxel de margen es un píxel menos de comida a la vista, y esta
    // pantalla vive en 600 px de alto.
    <Box p={3} h="100%" overflowY="auto">
      {/* Encabezado de un solo renglón. Antes ocupaba dos con el título en xl. */}
      <HStack mb={3} gap={2} flexWrap="wrap">
        <Text fontSize="lg" fontWeight="800">Pedidos</Text>
        <Badge colorPalette={live ? 'green' : 'gray'}>{live ? 'En vivo' : 'Sin conexión'}</Badge>
        {pendiente.cuantos > 0 && (
          <Badge colorPalette="orange" px={2} py={1}>
            {pendiente.cuantos} por cobrar · {money(String(pendiente.monto))}
          </Badge>
        )}
      </HStack>

      {/* Dos columnas en pantalla ancha, una sola abajo de 900 px: en una tableta de 7" dos
          columnas dejan tarjetas donde el nombre del producto ya no cabe. */}
      <SimpleGrid columns={{ base: 1, md: 2 }} gap={3} alignItems="start">
        <Columna titulo="En preparación" orders={preparando} acciones={acciones} />
        <Columna titulo="A entregar" orders={listos} acciones={acciones} />
      </SimpleGrid>

      {canRefund && (
        <Entregadas orders={entregadas} onRefund={refund} onTicket={acciones.ticket}
          onCobrar={setCobrando} puedeCobrar={puedeCobrar} />
      )}

      <ReprintTicket orderId={ticketOrderID} onClose={() => setTicketOrderID(null)} />
      <CobrarSheet order={cobrando} onClose={() => setCobrando(null)} onCobrado={invalidateAll} />
    </Box>
  );
}

interface Acciones {
  // puedeCobrar viene del ajuste del negocio: apagado, este tablero no toca dinero.
  puedeCobrar: boolean;
  entregarLinea: (id: number, lineId: number, qty: number) => void;
  entregarTodo: (o: BoardOrder) => void;
  cobrar: (o: BoardOrder) => void;
  ticket: (o: BoardOrder) => void;
  cancelar: (o: BoardOrder) => void;
}

function Columna({ titulo, orders, acciones }: { titulo: string; orders: BoardOrder[]; acciones: Acciones }) {
  return (
    <Box>
      <HStack mb={2} gap={2}>
        <Text fontWeight="700">{titulo}</Text>
        <Badge borderRadius="full" px={2}>{orders.length}</Badge>
      </HStack>
      <VStack align="stretch" gap={2}>
        {orders.length === 0 && <Text color="fg.subtle" fontSize="sm">Sin pedidos</Text>}
        {orders.map((o) => <Tarjeta key={o.id} o={o} acciones={acciones} />)}
      </VStack>
    </Box>
  );
}

// La tarjeta de un pedido, con sus productos SIEMPRE a la vista.
//
// Antes venían plegados detrás de un tap. Lo que falta por entregar es justo lo que el operador
// vino a leer: esconderlo le cobraba un tap por pedido y dejaba la tarjeta llena de encabezado.
function Tarjeta({ o, acciones }: { o: BoardOrder; acciones: Acciones }) {
  const Svc = SERVICE_META[o.serviceType]?.icon;
  const faltan = pendientes(o);
  const listo = faltan.length === 0;
  const debe = Number(o.outstanding) > 0;

  return (
    <Box bg="bg.panel" borderWidth="1px" borderColor={debe ? 'orange.300' : 'border'} borderRadius="lg" p={2.5}>
      <Flex justify="space-between" align="start" gap={2} mb={2}>
        <Box minW={0}>
          {/* El nombre manda: es con lo que se canta el pedido. El número queda en el renglón de
              abajo, junto a lo demás que solo se consulta. */}
          <Text fontWeight="800" fontSize="lg" lineHeight="1.2" lineClamp={1}>
            {o.folioName || `#${o.number}`}
          </Text>
          <HStack fontSize="xs" color="fg.muted" gap={1}>
            {Svc && <Svc size={12} />}
            <Text as="span" lineClamp={1}>
              #{o.number} · {SERVICE_META[o.serviceType]?.label ?? o.serviceType}
              {o.customerName ? ` · ${o.customerName}` : ''}
              {o.lines.length > 1 ? ` · ${entregados(o)}/${o.lines.length}` : ''}
            </Text>
          </HStack>
        </Box>
        <VStack align="end" gap={0} flexShrink={0}>
          <Text fontWeight="700" lineHeight="1.2">{money(o.total, o.currency)}</Text>
          {debe && (
            <Text fontSize="xs" fontWeight="700" color="orange.600">
              debe {money(o.outstanding, o.currency)}
            </Text>
          )}
        </VStack>
      </Flex>

      <VStack align="stretch" gap={1} mb={2}>
        {faltan.map((l) => (
          // La clave lleva lo ya entregado a propósito: el contador del renglón es estado local, y
          // sin esto seguiría en 3 después de entregar 3 de 5 — el botón mandaría al servidor una
          // cantidad mayor a la que falta y el operador vería un error por haber acertado.
          <Renglon key={`${l.id}-${l.delivered}`} l={l}
            onEntregar={(qty) => acciones.entregarLinea(o.id, l.id, qty)} />
        ))}
        {listo && (
          <HStack color="green.600" py={1} gap={1}>
            <LuCheck size={16} />
            <Text fontSize="sm" fontWeight="600">Todo entregado</Text>
          </HStack>
        )}
      </VStack>

      <HStack gap={2}>
        {/* Entregar todo desaparece cuando ya no falta nada: un botón que no hace nada enseña a
            ignorar el que sí hace. */}
        {!listo && (
          <Button flex="1" minH={TAP} colorPalette="green" onClick={() => acciones.entregarTodo(o)}>
            Entregar todo
          </Button>
        )}
        {debe && acciones.puedeCobrar && (
          <Button flex="1" minH={TAP} colorPalette="orange" variant={listo ? 'solid' : 'outline'}
            onClick={() => acciones.cobrar(o)}>
            Cobrar
          </Button>
        )}
        <MenuRoot>
          <MenuTrigger asChild>
            <IconButton aria-label="Más" variant="outline" minH={TAP} minW={TAP}>
              <LuEllipsisVertical />
            </IconButton>
          </MenuTrigger>
          <MenuContent>
            <MenuItem value="ticket" onClick={() => acciones.ticket(o)}>Ver ticket</MenuItem>
            <MenuItem value="cancel" color="red.500" onClick={() => acciones.cancelar(o)}>Cancelar pedido</MenuItem>
          </MenuContent>
        </MenuRoot>
      </HStack>
    </Box>
  );
}

// Un producto que todavía debe salir.
//
// El botón verde entrega TODO lo que falta con un tap, que es el caso de siempre. El contador solo
// aparece cuando falta más de uno: es la excepción —salen 3 de 5 alitas y las otras 2 siguen en la
// freidora— y cobrarle un tap al caso común para servir a la excepción está al revés.
function Renglon({ l, onEntregar }: { l: BoardLine; onEntregar: (qty: number) => void }) {
  const falta = faltante(l);
  const [cantidad, setCantidad] = useState(falta);
  const parcial = falta > 1;
  const extras = [...(l.modifiers ?? []), ...(l.notes ? [l.notes] : [])];

  return (
    <HStack gap={1.5} align="center" borderWidth="1px" borderColor="border" borderRadius="md" px={1.5} py={1}>
      <Text fontWeight="800" fontSize="sm" minW="1.75rem" textAlign="center" flexShrink={0}>
        {Number(l.qty)}
      </Text>
      <Box flex="1" minW={0}>
        <Text fontWeight="600" fontSize="sm" lineHeight="1.25" lineClamp={1}>{l.name}</Text>
        {/* En una cocina "Alitas" y "Alitas BBQ sin cebolla" son platillos distintos. Sin esto la
            tarjeta no alcanza a reemplazar la libreta. */}
        {extras.length > 0 && (
          <Text fontSize="2xs" color="fg.muted" lineHeight="1.25" lineClamp={1}>{extras.join(' · ')}</Text>
        )}
        {Number(l.delivered) > 0 && (
          <Text fontSize="2xs" color="orange.600" lineHeight="1.25">
            salieron {Number(l.delivered)} de {Number(l.qty)}
          </Text>
        )}
      </Box>
      {parcial && (
        <HStack gap={0.5} flexShrink={0}>
          <IconButton aria-label="Uno menos" size="sm" variant="ghost" minH={TAP} minW="2rem"
            disabled={cantidad <= 1} onClick={() => setCantidad((c) => Math.max(1, c - 1))}>
            <LuMinus />
          </IconButton>
          <Text minW="1.25rem" textAlign="center" fontWeight="700" fontSize="sm">{cantidad}</Text>
          <IconButton aria-label="Uno más" size="sm" variant="ghost" minH={TAP} minW="2rem"
            disabled={cantidad >= falta} onClick={() => setCantidad((c) => Math.min(falta, c + 1))}>
            <LuPlus />
          </IconButton>
        </HStack>
      )}
      <Button size="sm" minH={TAP} px={3} colorPalette="green" flexShrink={0}
        onClick={() => onEntregar(parcial ? cantidad : falta)}>
        <LuCheck />
      </Button>
    </HStack>
  );
}

// Entregadas del día: solo admin/gerente, para reembolsar y para cobrar lo que quedó pendiente.
// TOPADA para no competir con el flujo operativo de arriba: en una jornada llena son decenas.
function Entregadas({ orders, onRefund, onTicket, onCobrar, puedeCobrar }: {
  orders: BoardOrder[];
  onRefund: (o: BoardOrder) => void;
  onTicket: (o: BoardOrder) => void;
  onCobrar: (o: BoardOrder) => void;
  puedeCobrar: boolean;
}) {
  return (
    <Box mt={4}>
      <HStack mb={2} gap={2}>
        <Text fontWeight="700">Entregadas hoy</Text>
        <Badge borderRadius="full" px={2}>{orders.length}</Badge>
        {orders.length > ENTREGADAS_VISIBLES && (
          <Text fontSize="xs" color="fg.muted">últimas {ENTREGADAS_VISIBLES}</Text>
        )}
      </HStack>
      {orders.length === 0 ? (
        <Text color="fg.subtle" fontSize="sm">Sin entregas hoy</Text>
      ) : (
        <VStack align="stretch" gap={1.5}>
          {orders.slice(0, ENTREGADAS_VISIBLES).map((o) => {
            const debe = Number(o.outstanding) > 0;
            return (
              <Flex key={o.id} bg="bg.panel" borderWidth="1px"
                borderColor={debe ? 'orange.300' : 'border'} borderRadius="lg"
                px={3} py={1.5} justify="space-between" align="center" gap={2}>
                <Box minW={0}>
                  <Text fontWeight="700" lineHeight="1.2" lineClamp={1}>{o.folioName || `#${o.number}`}</Text>
                  <Text fontSize="xs" color="fg.muted" lineClamp={1}>
                    #{o.number} · {SERVICE_META[o.serviceType]?.label ?? o.serviceType}
                    {o.customerName ? ` · ${o.customerName}` : ''}
                  </Text>
                </Box>
                <HStack gap={2} flexShrink={0}>
                  <Text fontWeight="700">{money(o.total, o.currency)}</Text>
                  {/* Aquí es donde el pendiente deja de tener remedio: el cliente ya se fue con la
                      comida. Por eso el botón de cobrar vive junto al aviso y no en otra pantalla. */}
                  {debe && puedeCobrar && (
                    <Button size="sm" minH={TAP} colorPalette="orange" onClick={() => onCobrar(o)}>
                      Cobrar {money(o.outstanding, o.currency)}
                    </Button>
                  )}
                  <Button size="sm" minH={TAP} variant="outline" onClick={() => onTicket(o)}>Ticket</Button>
                  <Button size="sm" minH={TAP} variant="outline" colorPalette="red"
                    onClick={() => onRefund(o)}>Reembolsar</Button>
                </HStack>
              </Flex>
            );
          })}
        </VStack>
      )}
    </Box>
  );
}
