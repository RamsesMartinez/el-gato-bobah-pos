import { useQuery } from '@tanstack/react-query';

import { posApi } from '../../api/pos';
import { TicketPreview } from './TicketPreview';

// ReprintTicket abre el ticket de un pedido ya cerrado. Pide el pedido COMPLETO porque la lista del
// tablero trae solo la cabecera: sin las líneas, el ticket saldría vacío.
export function ReprintTicket({ orderId, onClose }: { orderId: number | null; onClose: () => void }) {
  const { data } = useQuery({
    queryKey: ['order', orderId],
    queryFn: () => posApi.order(orderId as number),
    enabled: orderId !== null,
  });

  // reprint marca el papel. No es cosmético: sin la marca, dos tickets idénticos del mismo pedido
  // pueden circular como si fueran ventas distintas.
  return <TicketPreview order={data ?? null} reprint isOpen={orderId !== null} onClose={onClose} />;
}
