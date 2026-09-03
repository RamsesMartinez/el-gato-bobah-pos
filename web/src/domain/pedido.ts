import type { CreateOrderBody } from '../types/pos';
import type { TicketLine } from '../types/pos';
import type { TicketTab } from '../types/pos';

// Cómo se traduce una cuenta del POS al cuerpo que espera el servidor, y las reglas del pedido que
// la pantalla necesita para no contradecirlo.
//
// Vive en `domain` —sin React, sin Chakra, sin api— porque es la regla, no la pantalla. Todo el que
// necesite saber si un pedido cobra envío pregunta aquí; deducirlo por su cuenta es como la pantalla
// llegó a ofrecer un envío que el servidor no cobra.
//
// El servidor recalcula todos los precios: de aquí solo viajan ids, cantidades y con qué lista.

// cobraEnvio dice si ESTE pedido lleva el costo de envío del negocio.
//
// Una plataforma reparte con su propia gente y cobra ese reparto aparte, así que el envío del
// negocio no aplica aunque la cuenta esté marcada como domicilio — y esa combinación es alcanzable:
// se marca domicilio primero y se asigna la plataforma después, momento en el que el panel del
// pedido ya escondió los botones de tipo y el operador no puede corregirlo.
//
// La regla vive aquí y no repetida en cada pantalla porque el servidor la aplica al crear el pedido
// (`cobraEnvio` en app/orders.go): una segunda deducción es una segunda oportunidad de divergir, y
// cuando divergió la pantalla ofrecía cobrar $115 de un pedido de $95.
export function cobraEnvio(cuenta: Pick<TicketTab, 'serviceType' | 'platformId'>): boolean {
  return cuenta.serviceType === 'domicilio' && cuenta.platformId === null;
}

export interface ArmarPedidoInput {
  cuenta: TicketTab;
  // Renglones que sí se pueden cobrar. El llamador ya quitó los productos que se inactivaron
  // mientras estaban en el carrito.
  lineas: TicketLine[];
  clientUuid: string;
  // Costo de envío del negocio. Solo aplica a domicilio propio.
  deliveryFee: number;
}

export function armarPedido({ cuenta, lineas, clientUuid, deliveryFee }: ArmarPedidoInput): CreateOrderBody {
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
    // La MISMA regla que usa la pantalla para decidir qué total pinta. Escrita dos veces, ya
    // divergió una vez.
    deliveryFee: cobraEnvio(cuenta) ? deliveryFee : 0,
    deliveryPlatformId: lista ?? undefined,
    lines: lineas.map((l) => ({
      productId: l.productId,
      qty: l.qty,
      notes: l.notes,
      modifiers: l.modifiers.map((m) => ({ optionId: m.optionId, qty: m.qty })),
    })),
  };
}
