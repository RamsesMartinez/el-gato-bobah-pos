import { useEffect, useRef } from 'react';

import { buildReceiptHtml, printHtmlOffscreen } from '../../utils/printReceipt';
import { toaster } from '../../components/ui/toaster';
import { useTicketBusinessInfo } from './ticketBusinessInfo';
import type { OrderView } from '../../types/pos';

// AutoPrintTicket no dibuja nada: saca el ticket del pedido recién cerrado cuando el negocio activó
// la impresión automática. Vive como componente y no como llamada suelta para que el efecto se
// cancele solo si el POS se desmonta a media venta.
export function AutoPrintTicket({ order }: { order: OrderView | null }) {
  const { data: business, autoPrintOnClose } = useTicketBusinessInfo();
  // Se recuerda el pedido ya impreso, no un simple "ya imprimí": React puede re-renderizar por
  // cualquier motivo, y cada re-render que imprimiera sería un ticket duplicado en la mano del
  // cliente.
  const printedOrderID = useRef<number | null>(null);

  useEffect(() => {
    if (!order || !business || !autoPrintOnClose) return;
    if (printedOrderID.current === order.id) return;
    printedOrderID.current = order.id;
    // Sin await a propósito: el pedido YA está registrado, y una impresora apagada no debe trabar
    // la pantalla ni perder la venta. Pero tampoco puede fallar en silencio — que no salga papel y
    // nadie se entere es justo el modo de fallo que esta feature vino a quitar.
    void printHtmlOffscreen(buildReceiptHtml(order, business, {})).then((printed) => {
      if (printed) return;
      toaster.create({
        title: 'No se pudo imprimir el ticket',
        description: 'Ábrelo con «Ver ticket» e imprímelo desde ahí. La venta ya quedó registrada.',
        type: 'warning',
      });
    });
  }, [order, business, autoPrintOnClose]);

  return null;
}
