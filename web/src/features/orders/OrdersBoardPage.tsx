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

const SERVICE_META: Record<string, { label: string; icon: IconType }> = {
  mostrador: { label: 'Mostrador', icon: LuStore },
  para_llevar: { label: 'Llevar', icon: LuShoppingBag },
  domicilio: { label: 'Domicilio', icon: LuBike },
};

const CANCEL_REASONS = ['Cliente canceló', 'Error de captura', 'Sin insumos', 'Otro'];

export function OrdersBoardPage() {
  const live = useOrderEvents();
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: ['orders', 'active'],
    queryFn: posApi.activeOrders,
    refetchInterval: 10_000, // respaldo si SSE se cae
  });

  const invalidate = () => qc.invalidateQueries({ queryKey: ['orders', 'active'] });
  const statusMut = useMutation({
    mutationFn: ({ id, status }: { id: number; status: string }) => posApi.setOrderStatus(id, status),
    onSuccess: invalidate,
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });
  const cancelMut = useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) => posApi.cancelOrder(id, reason),
    onSuccess: invalidate,
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

  return (
    <Box p={4} h="100%" overflowY="auto">
      <HStack mb={4}>
        <Text fontSize="xl" fontWeight="800">Pedidos activos</Text>
        <Badge colorPalette={live ? 'green' : 'gray'}>{live ? 'En vivo' : 'Sin conexión'}</Badge>
      </HStack>
      <SimpleGrid columns={{ base: 1, md: 2 }} gap={4}>
        <Column title="En preparación" orders={preparando} onAdvance={advance} onCancel={cancel} advanceLabel="Marcar listo" />
        <Column title="Listos" orders={listos} onAdvance={advance} onCancel={cancel} advanceLabel="Entregar" />
      </SimpleGrid>
    </Box>
  );
}

interface ColProps {
  title: string;
  orders: BoardOrder[];
  advanceLabel: string;
  onAdvance: (o: BoardOrder) => void;
  onCancel: (o: BoardOrder) => void;
}

function Column({ title, orders, advanceLabel, onAdvance, onCancel }: ColProps) {
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
                <Text fontWeight="700">{money(o.total)}</Text>
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
