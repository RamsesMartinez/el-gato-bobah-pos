import type { CreateOrderBody } from '../../api/pos';
import type { TicketLine } from '../../types/pos';
import type { TicketTab } from '../../stores/ticket';

// Cómo se traduce una cuenta del POS al cuerpo que espera el servidor.
//
// Vive fuera de los componentes porque ahora lo usan DOS pantallas: el panel del pedido, que manda
// a cocina sin cobrar, y la hoja de cobro. Antes solo lo usaba la hoja, y por eso mandar a cocina
// obligaba a abrir una pantalla llena de controles de dinero que después se descartaban.
//
// El servidor recalcula todos los precios: de aquí solo viajan ids, cantidades y con qué lista.

export interface ArmarPedidoInput {
  cuenta: TicketTab;
  // Renglones que sí se pueden cobrar. El llamador ya quitó los productos que se inactivaron
  // mientras estaban en el carrito.
  lineas: TicketLine[];
  clientUuid: string;
  // Costo de envío del negocio. Solo aplica a domicilio propio.
  deliveryFee: number;
  payments?: CreateOrderBody['payments'];
}

export function armarPedido({ cuenta, lineas, clientUuid, deliveryFee, payments }: ArmarPedidoInput): CreateOrderBody {
  const lista = cuenta.platformId;
  return {
    clientUuid,
    // El animal que la cuenta lleva mostrando desde que se abrió. Se manda para que el ticket salga
    // con el mismo nombre que el operador ya le dijo al cliente; el servidor lo sanea y le agrega la
    // vuelta si otro pedido del día se le adelantó.
    folioName: cuenta.folioName,
    // Un pedido de plataforma ES a domicilio: lo reparte la plataforma. El servidor lo exige por el
    // check de la tabla, así que la pantalla no puede mandar otra cosa.
    serviceType: lista !== null ? 'domicilio' : cuenta.serviceType,
    customerName: cuenta.customerName || undefined,
    // El envío del negocio no aplica a una plataforma: lo cobra ella. El servidor también lo fuerza
    // a 0, pero mandarlo ya en 0 evita mostrar un total que el servidor no va a cobrar.
    deliveryFee: lista !== null ? 0 : deliveryFee,
    deliveryPlatformId: lista ?? undefined,
    lines: lineas.map((l) => ({
      productId: l.productId,
      qty: l.qty,
      notes: l.notes,
      modifiers: l.modifiers.map((m) => ({ optionId: m.optionId, qty: m.qty })),
    })),
    // Sin pagos = mandado a cocina y por cobrar. El servidor lo deja abierto y el tablero lo marca.
    payments,
  };
}
