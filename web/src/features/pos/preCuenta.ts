import type { ReceiptOrder, TicketLine } from '../../types/pos';
import { lineTotal } from '../../stores/ticket';

// Arma el papel de una cuenta que TODAVÍA NO es un pedido.
//
// El papel sale de la misma impresora que los comprobantes de venta, así que lo que aquí se decide
// es qué lo distingue de uno: no lleva número de pedido —no existe— y quien lo imprime le pone la
// marca ** PRE-CUENTA **. Ver specs/012-imprimir-la-cuenta.
//
// Vive aparte del componente y es pura a propósito: convertir una cuenta en un papel es una
// decisión sobre dinero, y una decisión se prueba.
export function preCuentaDeLaCuenta(
  { folioName, serviceType, customerName, lineas, envio, total }: {
    folioName: string;
    serviceType: string;
    customerName: string;
    // Las líneas COBRABLES, no las del carrito completo: la pantalla ya excluye los productos que
    // dejaron de existir, y el papel tiene que mostrar lo que se va a cobrar, no lo que se capturó.
    lineas: TicketLine[];
    envio: number;
    total: number;
  },
  ahora: Date,
): ReceiptOrder {
  const subtotal = total - envio;
  return {
    id: 0,
    // El número NO existe todavía. Se manda en cero y quien imprime la pre-cuenta no lo pinta;
    // inventar uno sería peor, porque el operador se lo diría al cliente y no coincidiría.
    number: 0,
    // El nombre SÍ va: la pantalla lo propone de la misma bolsa de la que el servidor reparte, así
    // que coincide casi siempre. Es un techo conocido — si otra estación lo toma antes, el papel
    // impreso queda con uno distinto al del ticket. Lo cierra la 013, cuando la orden nazca al
    // primer producto y el nombre quede amarrado.
    folioName,
    status: 'abierta',
    serviceType,
    customerName: customerName || null,
    subtotal: subtotal.toFixed(2),
    deliveryFee: envio.toFixed(2),
    total: total.toFixed(2),
    currency: 'MXN',
    // `paid` no se imprime en una pre-cuenta —el estado del cobro es justo lo que la confundiría con
    // el ticket de un pedido real sin cobrar— pero el tipo lo pide.
    paid: false,
    refund: '0',
    openedAt: ahora.toISOString(),
    lines: lineas.map((l) => ({
      productName: l.name,
      quantity: String(l.qty),
      unitPrice: l.unitPrice.toFixed(2),
      lineTotal: lineTotal(l).toFixed(2),
      notes: l.notes,
      modifiers: l.modifiers.map((m) => ({
        name: m.name,
        quantity: m.qty,
        priceDelta: (m.priceDelta ?? 0).toFixed(2),
      })),
    })),
  };
}
