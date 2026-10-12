import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Box, Button, HStack, Spinner, Text, VStack } from '@chakra-ui/react';
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

// movedTo dice a qué pedido se fueron (o de cuál vinieron) los productos de un traslado. El id es
// interno y no se pinta: lo que el operador necesita es poder abrir el otro historial.
function movedTo(e: AuditEvent): { orderId: number; text: string } | null {
  if (e.action !== 'order.lines_moved') return null;
  const after = e.after ?? {};
  const n = Number(after.lines ?? 0);
  const products = n === 1 ? '1 producto' : `${n} productos`;
  if (typeof after.toOrderId === 'number') return { orderId: after.toOrderId, text: `Se pasaron ${products} a otro pedido` };
  if (typeof after.fromOrderId === 'number') return { orderId: after.fromOrderId, text: `Llegaron ${products} de otro pedido` };
  return null;
}

// OrderHistoryTimeline es el historial de un pedido: quién hizo qué, cuándo y desde dónde. Lo ve solo
// quien tiene el permiso; quien lo monta ya preguntó.
export function OrderHistoryTimeline({ orderId }: { orderId: number }) {
  // Seguir un traslado abre el otro historial aquí mismo; la pila deja regresar sin cerrar la hoja.
  const [trail, setTrail] = useState<number[]>([]);
  const current = trail.length > 0 ? trail[trail.length - 1] : orderId;
  // Al traslado se llega bajando por el historial; el otro pedido abre desde arriba, donde está
  // «Volver», y no a media hoja.
  const top = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (trail.length > 0) top.current?.scrollIntoView?.({ block: 'start' });
  }, [trail]);
  return (
    <VStack ref={top} align="stretch" gap={2}>
      {trail.length > 0 && (
        <Button minH="44px" variant="outline" alignSelf="flex-start" onClick={() => setTrail((t) => t.slice(0, -1))}>
          Volver al pedido anterior
        </Button>
      )}
      <Timeline key={current} orderId={current} onOpenOrder={(id) => setTrail((t) => [...t, id])} />
    </VStack>
  );
}

function Timeline({ orderId, onOpenOrder }: { orderId: number; onOpenOrder: (id: number) => void }) {
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
      {events.map((e) => {
        const moved = movedTo(e);
        return (
        <HStack key={e.id} align="flex-start" gap={3} py={2} borderBottomWidth="1px">
          <Text fontSize="sm" color="fg.muted" w="80px" flexShrink={0} whiteSpace="nowrap">{soloHora(e.occurredAt, zone)}</Text>
          <Box w="10px" h="10px" borderRadius="full" bg={dotColor(e.action)} mt="6px" flexShrink={0} />
          <Box minW={0}>
            <Text fontWeight="700">{actionLabel(e.action)}</Text>
            <Text fontSize="sm" color="fg.muted">
              {[e.actorName, moved?.text ?? summarizeChange(e.before, e.after)].filter(Boolean).join(' · ')}
            </Text>
            {moved && (
              <Button mt={1} minH="44px" size="sm" variant="outline" onClick={() => onOpenOrder(moved.orderId)}>
                Ver el otro pedido
              </Button>
            )}
            {/* La IP no va en la línea de tiempo: se lee en el detalle de la bitácora, que es para
                auditar. Aquí basta saber desde qué aparato. */}
            {(deviceLabel(e) || e.deviceKey) && (
              <Text fontSize="xs" color="fg.muted">
                {[deviceLabel(e), e.deviceKey && `Aparato ${e.deviceKey.slice(0, 4).toUpperCase()}`].filter(Boolean).join(' · ')}
              </Text>
            )}
          </Box>
        </HStack>
        );
      })}
    </VStack>
  );
}
