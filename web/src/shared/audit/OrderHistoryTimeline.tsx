import { useQuery } from '@tanstack/react-query';
import { Box, HStack, Spinner, Text, VStack } from '@chakra-ui/react';
import { auditApi, type AuditEvent } from '../../api/audit';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { soloHora } from '../../utils/horaDelNegocio';
import { actionLabel, summarizeChange } from './labels';

// El color del punto dice de un vistazo qué fue: entra dinero, sale dinero, o lo demás.
function dotColor(action: string): string {
  if (action === 'payment.charged') return 'green.500';
  if (['payment.voided', 'payment.refunded', 'order.cancelled', 'order.line_removed', 'stock.written_off'].includes(action)) return 'red.500';
  return 'gray.400';
}

function deviceLabel(e: AuditEvent): string {
  const d = e.device ?? {};
  return [d.platform, d.browser, d.model].filter(Boolean).join(' · ');
}

// OrderHistoryTimeline es el historial de un pedido: quién hizo qué, cuándo y desde dónde. Lo ve solo
// quien tiene el permiso; quien lo monta ya preguntó.
export function OrderHistoryTimeline({ orderId }: { orderId: number }) {
  const { zona: zone } = useHoraDelNegocio();
  const { data, isLoading, isError } = useQuery({
    queryKey: ['audit', 'order', orderId],
    queryFn: () => auditApi.orderHistory(orderId),
  });
  if (isLoading) return <HStack><Spinner size="sm" /><Text color="fg.muted">Cargando historial…</Text></HStack>;
  if (isError) return <Text color="red.fg">No se pudo cargar el historial.</Text>;
  const events = data ?? [];
  if (events.length === 0) {
    return <Text color="fg.muted">No hay acciones registradas de este pedido.</Text>;
  }
  return (
    <VStack align="stretch" gap={0}>
      {events.map((e) => (
        <HStack key={e.id} align="flex-start" gap={3} py={2} borderBottomWidth="1px">
          <Text fontSize="sm" color="fg.muted" w="64px" flexShrink={0}>{soloHora(e.occurredAt, zone)}</Text>
          <Box w="10px" h="10px" borderRadius="full" bg={dotColor(e.action)} mt="6px" flexShrink={0} />
          <Box minW={0}>
            <Text fontWeight="700">{actionLabel(e.action)}</Text>
            <Text fontSize="sm" color="fg.muted">
              {[e.actorName, summarizeChange(e.before, e.after)].filter(Boolean).join(' · ')}
            </Text>
            {/* La IP no va en la línea de tiempo: se lee en el detalle de la bitácora, que es para
                auditar. Aquí basta saber desde qué aparato. */}
            {(deviceLabel(e) || e.deviceKey) && (
              <Text fontSize="xs" color="fg.muted">
                {[deviceLabel(e), e.deviceKey && `Aparato ${e.deviceKey.slice(0, 4).toUpperCase()}`].filter(Boolean).join(' · ')}
              </Text>
            )}
          </Box>
        </HStack>
      ))}
    </VStack>
  );
}
