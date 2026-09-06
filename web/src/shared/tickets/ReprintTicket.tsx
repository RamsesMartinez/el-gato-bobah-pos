import { useQuery } from '@tanstack/react-query';

import { posApi } from '../../api/pos';
import { TicketPreview } from './TicketPreview';

// VerTicket abre el ticket de un pedido por su id. Pide el pedido COMPLETO porque las listas traen
// solo la cabecera: sin las líneas, el ticket saldría vacío.
//
// La marca de REIMPRESIÓN se DERIVA de si el pedido ya está pagado, no se pasa por fuera.
//
// El porqué: la marca existe para que dos papeles del mismo pedido no circulen como si fueran dos
// ventas distintas. Un pedido pagado ya sacó su ticket al cobrarse, así que abrirlo otra vez es una
// reimpresión y hay que decirlo. Uno que todavía NO se ha cobrado nunca sacó ticket de venta: ese
// papel es la cuenta, lleva impreso "POR COBRAR" y no puede pasar por un comprobante de venta.
// Marcarlo como reimpresión sería mentir sobre algo que el cliente tiene en la mano.
//
// Derivarlo en vez de recibirlo como prop es a propósito: es una regla, y una regla que viaja como
// parámetro se pasa mal desde la tercera pantalla que la usa.
export function VerTicket({ orderId, onClose }: { orderId: number | null; onClose: () => void }) {
  const { data } = useQuery({
    queryKey: ['order', orderId],
    queryFn: () => posApi.order(orderId as number),
    enabled: orderId !== null,
  });

  return (
    <TicketPreview
      order={data ?? null}
      reprint={data?.paid ?? false}
      isOpen={orderId !== null}
      onClose={onClose}
    />
  );
}

// ReprintTicket es el nombre con el que el tablero ya la llamaba. Se conserva para no tocar esa
// pantalla en un cambio que no es suyo.
export const ReprintTicket = VerTicket;
