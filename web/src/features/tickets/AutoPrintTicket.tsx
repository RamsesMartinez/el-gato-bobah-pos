import { useEffect, useRef } from 'react';

import { buildReceiptHtml, printHtmlOffscreen } from '../../utils/printReceipt';
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
    // Sin await ni manejo de error a propósito: el pedido YA está registrado, y una impresora
    // apagada no debe trabar la pantalla ni perder la venta. Se reimprime desde el tablero.
    void printHtmlOffscreen(buildReceiptHtml(order, business, {}));
  }, [order, business, autoPrintOnClose]);

  return null;
}
