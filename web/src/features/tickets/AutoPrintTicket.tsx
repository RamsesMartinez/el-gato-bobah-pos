import { useEffect, useRef } from 'react';

import { buildKitchenHtml } from '../../utils/printKitchen';
import { buildReceiptHtml, printHtmlOffscreen } from '../../utils/printReceipt';
import { toaster } from '../../components/ui/toaster';
import { useTicketBusinessInfo } from './ticketBusinessInfo';
import type { ReceiptOrder } from '../../types/pos';

// AutoPrintTicket no dibuja nada: saca el ticket del pedido recién cerrado cuando el negocio activó
// la impresión automática. Vive como componente y no como llamada suelta para que el efecto se
// cancele solo si el POS se desmonta a media venta.
export function AutoPrintTicket({ order }: { order: ReceiptOrder | null }) {
  const { data: business, autoPrintOnClose, printFreeModifiers } = useTicketBusinessInfo();
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
    void printHtmlOffscreen(buildReceiptHtml(order, business, { printFreeModifiers })).then((printed) => {
      if (printed) return;
      toaster.create({
        title: 'No se pudo imprimir el ticket',
        description: 'Ábrelo con «Ver ticket» e imprímelo desde ahí. La venta ya quedó registrada.',
        type: 'warning',
      });
    });
  }, [order, business, autoPrintOnClose, printFreeModifiers]);

  return null;
}

// KitchenTicket saca la COMANDA —el papel sin precios— del pedido recién mandado, si el negocio la
// activó. Va aparte de AutoPrintTicket y no como una bandera dentro: son dos documentos distintos,
// para dos personas distintas, con dos ajustes que se encienden por separado. Meterlos en el mismo
// componente obligaría a leer un `if` para saber cuál sale.
export function KitchenTicket({ order }: { order: ReceiptOrder | null }) {
  const { printKitchenTicket } = useTicketBusinessInfo();
  // Igual que el ticket del cliente: se recuerda el pedido ya impreso, porque cada re-render que
  // imprimiera sería una comanda duplicada en la plancha.
  const impreso = useRef<number | null>(null);

  useEffect(() => {
    if (!order || !printKitchenTicket) return;
    if (impreso.current === order.id) return;
    impreso.current = order.id;
    void printHtmlOffscreen(buildKitchenHtml(order)).then((printed) => {
      if (printed) return;
      // Que no salga la comanda y nadie se entere es el modo de fallo que esto vino a quitar: sin
      // el aviso, cocina no prepara el pedido y nadie sabe por qué.
      toaster.create({
        title: 'No salió la comanda',
        description: 'Revisa la impresora. El pedido ya está registrado y se ve en Pedidos.',
        type: 'warning',
      });
    });
  }, [order, printKitchenTicket]);

  return null;
}
