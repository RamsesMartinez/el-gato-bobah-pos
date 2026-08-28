import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Box, SimpleGrid, Text, Badge, HStack, VStack, Center, Spinner, Flex, Button, IconButton,
} from '@chakra-ui/react';
import { MenuRoot, MenuTrigger, MenuContent, MenuItem } from '../../components/ui/menu';
import { LuStore, LuShoppingBag, LuBike, LuEllipsisVertical } from 'react-icons/lu';
import type { IconType } from 'react-icons';
import { toaster } from '../../components/ui/toaster';
import { posApi } from '../../api/pos';
import type { BoardOrder } from '../../types/pos';
import { money } from '../../utils/format';
import { useOrderEvents } from '../../hooks/useOrderEvents';
import { ReprintTicket } from '../tickets/ReprintTicket';
import { useSessionStore } from '../../stores/session';

const SERVICE_META: Record<string, { label: string; icon: IconType }> = {
  mostrador: { label: 'Mostrador', icon: LuStore },
  para_llevar: { label: 'Llevar', icon: LuShoppingBag },
  domicilio: { label: 'Domicilio', icon: LuBike },
};

const CANCEL_REASONS = ['Cliente canceló', 'Error de captura', 'Sin insumos', 'Otro'];
const REFUND_REASONS = ['Producto mal', 'Se cayó / dañó', 'Queja del cliente', 'Cobro erróneo', 'Otro'];

export function OrdersBoardPage() {
  const live = useOrderEvents();
  // Pedido cuyo ticket se está viendo; null = ninguno.
  const [ticketOrderID, setTicketOrderID] = useState<number | null>(null);
  const qc = useQueryClient();
  // Reembolsar = salida de dinero → solo admin/gerente ven las entregadas y la acción. El
  // backend igual aplica el 403; esto es UX (no mostrar lo que no pueden usar).
  const role = useSessionStore((s) => s.user?.role);
  const canRefund = role === 'admin' || role === 'gerente';

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

  const invalidateActive = () => qc.invalidateQueries({ queryKey: ['orders', 'active'] });
  const invalidateAll = () => {
    invalidateActive();
    qc.invalidateQueries({ queryKey: ['orders', 'delivered'] });
  };
  const statusMut = useMutation({
    mutationFn: ({ id, status }: { id: number; status: string }) => posApi.setOrderStatus(id, status),
    onSuccess: invalidateAll, // entregar mueve la orden a la sección de entregadas
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });
  const cancelMut = useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) => posApi.cancelOrder(id, reason),
    onSuccess: invalidateActive,
  });
  const refundMut = useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) => posApi.refundOrder(id, reason),
    onSuccess: () => { invalidateAll(); toaster.create({ title: 'Reembolso registrado', type: 'success' }); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;
  const orders = data?.items ?? [];
  const preparando = orders.filter((o) => o.status === 'abierta');
  const listos = orders.filter((o) => o.status === 'lista');

  const advance = (o: BoardOrder) =>
    statusMut.mutate({ id: o.id, status: o.status === 'abierta' ? 'lista' : 'entregada' });
  const cancel = (o: BoardOrder) => {
    const reason = window.prompt(`Motivo de cancelación:\n(${CANCEL_REASONS.join(', ')})`, CANCEL_REASONS[0]);
    if (reason) cancelMut.mutate({ id: o.id, reason });
  };
  const showTicket = (o: BoardOrder) => setTicketOrderID(o.id);
  const refund = (o: BoardOrder) => {
    const reason = window.prompt(`Motivo del reembolso:\n(${REFUND_REASONS.join(', ')})`, REFUND_REASONS[0]);
    if (reason?.trim()) refundMut.mutate({ id: o.id, reason: reason.trim() });
  };

  return (
    <Box p={4} h="100%" overflowY="auto">
      <HStack mb={4}>
        <Text fontSize="xl" fontWeight="800">Pedidos activos</Text>
        <Badge colorPalette={live ? 'green' : 'gray'}>{live ? 'En vivo' : 'Sin conexión'}</Badge>
      </HStack>
      <SimpleGrid columns={{ base: 1, md: 2 }} gap={4}>
        <Column title="En preparación" orders={preparando} onAdvance={advance} onCancel={cancel} onTicket={showTicket} advanceLabel="Marcar listo" />
        <Column title="Listos" orders={listos} onAdvance={advance} onCancel={cancel} onTicket={showTicket} advanceLabel="Entregar" />
      </SimpleGrid>
      {canRefund && <DeliveredSection orders={deliveredData?.items ?? []} onRefund={refund} onTicket={showTicket} />}

      {/* Reimpresión: el ticket sale marcado para que no pase por un comprobante distinto. */}
      <ReprintTicket orderId={ticketOrderID} onClose={() => setTicketOrderID(null)} />
    </Box>
  );
}

// Entregadas del día: solo admin/gerente, para reembolsar (devolución = pérdida). Compacta
// para no competir con el flujo operativo de arriba.
function DeliveredSection({ orders, onRefund, onTicket }: { orders: BoardOrder[]; onRefund: (o: BoardOrder) => void; onTicket: (o: BoardOrder) => void }) {
  return (
    <Box mt={6}>
      <HStack mb={3}>
        <Text fontWeight="700" fontSize="lg">Entregadas hoy</Text>
        <Badge borderRadius="full" px={2}>{orders.length}</Badge>
      </HStack>
      {orders.length === 0 ? (
        <Text color="fg.subtle">Sin entregas hoy</Text>
      ) : (
        <VStack align="stretch" gap={2}>
          {orders.map((o) => (
            <Flex key={o.id} bg="bg.panel" borderWidth="1px" borderColor="border" borderRadius="lg"
              px={4} py={2} justify="space-between" align="center">
              <HStack gap={3}>
                <Text fontWeight="800">#{o.number}</Text>
                <Text fontSize="sm" color="fg.muted">
                  {SERVICE_META[o.serviceType]?.label ?? o.serviceType}{o.customerName ? ` · ${o.customerName}` : ''}
                </Text>
              </HStack>
              <HStack gap={3}>
                <Text fontWeight="700">{money(o.total, o.currency)}</Text>
                <Button size="sm" variant="outline" onClick={() => onTicket(o)}>Ticket</Button>
                <Button size="sm" variant="outline" colorPalette="red" onClick={() => onRefund(o)}>Reembolsar</Button>
              </HStack>
            </Flex>
          ))}
        </VStack>
      )}
    </Box>
  );
}

interface ColProps {
  title: string;
  orders: BoardOrder[];
  advanceLabel: string;
  onAdvance: (o: BoardOrder) => void;
  onCancel: (o: BoardOrder) => void;
  onTicket: (o: BoardOrder) => void;
}

function Column({ title, orders, advanceLabel, onAdvance, onCancel, onTicket }: ColProps) {
  return (
    <Box>
      <HStack mb={3}>
        <Text fontWeight="700" fontSize="lg">{title}</Text>
        <Badge borderRadius="full" px={2}>{orders.length}</Badge>
      </HStack>
      <VStack align="stretch" gap={3}>
        {orders.length === 0 && <Text color="fg.subtle">Sin pedidos</Text>}
        {orders.map((o) => {
          const Svc = SERVICE_META[o.serviceType]?.icon;
          return (
          <Box key={o.id} bg="bg.panel" borderWidth="1px" borderColor="border" borderRadius="lg" p={4}>
            <Flex justify="space-between" align="start" mb={3}>
              <Box>
                <Text fontWeight="800" fontSize="lg">#{o.number}</Text>
                <HStack fontSize="sm" color="fg.muted" gap={1}>
                  {Svc && <Svc size={14} />}
                  <Text as="span">
                    {SERVICE_META[o.serviceType]?.label ?? o.serviceType}{o.customerName ? ` · ${o.customerName}` : ''}
                  </Text>
                </HStack>
              </Box>
              <VStack align="end" gap={1}>
                <Text fontWeight="700">{money(o.total, o.currency)}</Text>
                {!o.paid && <Badge colorPalette="orange">POR COBRAR</Badge>}
              </VStack>
            </Flex>
            <HStack>
              <Button flex="1" size="md" onClick={() => onAdvance(o)}>
                {advanceLabel}
              </Button>
              <MenuRoot>
                <MenuTrigger asChild>
                  <IconButton aria-label="Más" variant="outline"><LuEllipsisVertical /></IconButton>
                </MenuTrigger>
                <MenuContent>
                  <MenuItem value="ticket" onClick={() => onTicket(o)}>Ver ticket</MenuItem>
                  <MenuItem value="cancel" color="red.500" onClick={() => onCancel(o)}>Cancelar pedido</MenuItem>
                </MenuContent>
              </MenuRoot>
            </HStack>
          </Box>
          );
        })}
      </VStack>
    </Box>
  );
}
