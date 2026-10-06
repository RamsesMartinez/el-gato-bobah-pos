import { api } from './client';

// Los pedidos que llegan de una plataforma de reparto (spec 021).
//
// LO QUE LA PANTALLA TIENE QUE SABER Y NO SE VE EN LOS TIPOS: la plataforma da 11.5 minutos para
// decidir antes de cancelar el pedido ella misma, y a los 90 segundos sin respuesta llama por
// teléfono al local. Por eso `decideBefore` viaja como instante absoluto y no como «minutos que
// quedan»: el reloj de la tableta puede estar corrido, y el plazo es del servidor.

/** Un platillo del pedido, con sus opciones colgando. */
export interface RenglonDePedido {
  externalName: string;
  quantity: string;
  unitPrice: string;
  productId: number | null;
  /** Si tiene pareja en el catálogo. `false` NO impide aceptar: el cliente ya pagó. */
  matched: boolean;
  /** Opcional a propósito: obliga a la guarda aunque el servidor prometa un arreglo. */
  options?: RenglonDePedido[];
}

export interface PedidoDePlataforma {
  id: number;
  platformName: string;
  /** El folio corto que ve el cliente. Es lo que dice por teléfono. */
  displayId: string;
  placedAt: string | null;
  /** Cuándo expira, en instante absoluto. */
  decideBefore: string | null;
  serviceType: string;
  customerName: string;
  total: string;
  /** Opcional a propósito. Un arreglo que el servidor promete puede llegar como `null` por un
   *  camino nuevo, y `.filter()` sobre `null` tumba la pantalla entera con la API en 200. Ya pasó. */
  lines?: RenglonDePedido[];
}

export interface PedidoAceptado {
  id: number;
  number: number;
  platformRef: string;
}

export const pedidosPendientes = () =>
  api.get<{ orders: PedidoDePlataforma[] }>('/orders/platform/pending').then((r) => r.orders ?? []);

export const aceptarPedidoDePlataforma = (id: number) =>
  api.post<PedidoAceptado>(`/orders/platform/${id}/accept`, {});
