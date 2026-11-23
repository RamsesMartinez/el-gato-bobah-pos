import { api } from './client';

// Pedidos entrantes (spec 093, contracts/incoming-orders.md): lo que llega de fuera y espera que el
// negocio lo acepte o lo rechace. Forma COMÚN a todos los orígenes: hoy la plataforma de reparto;
// el pedido web (#328) llega por aquí mismo.

export type OrigenDeEntrante = 'platform';

/** Un platillo con sus opciones colgando. */
export interface RenglonEntrante {
  name: string;
  quantity: string;
  unitPrice: string;
  /** Si tiene pareja en el catálogo. `false` no impide aceptar: el cliente ya pagó. */
  matched: boolean;
  /** Opcional a propósito: obliga a la guarda aunque el servidor prometa un arreglo. */
  options?: RenglonEntrante[];
}

export interface PedidoEntrante {
  source: OrigenDeEntrante;
  id: number;
  /** El nombre visible del origen: «Uber Eats», después «Web». */
  sourceName: string;
  /** El folio que ve el cliente. Es lo que dice por teléfono. */
  displayId: string;
  customerName: string;
  customerPhone: string;
  address: string;
  serviceType: string;
  total: string;
  placedAt: string | null;
  /** Cuándo vence, en instante absoluto del servidor. Nulo si el origen no tiene plazo. */
  decideBefore: string | null;
  /** Opcional a propósito: un `null` por un camino nuevo tumba la pantalla con la API en 200. */
  lines?: RenglonEntrante[];
}

export interface PedidoAceptado {
  id: number;
  number: number;
  platformRef: string;
}

export type MotivoDeRechazo = 'closed' | 'out_of_stock' | 'too_busy' | 'address' | 'other';

/** Los motivos en palabras de quien opera, en el orden en que se ofrecen. */
export const MOTIVOS_DE_RECHAZO: { code: MotivoDeRechazo; label: string }[] = [
  { code: 'closed', label: 'Ya cerramos' },
  { code: 'out_of_stock', label: 'Se acabó un producto' },
  { code: 'too_busy', label: 'Estamos saturados' },
  { code: 'address', label: 'No llegamos a esa dirección' },
  { code: 'other', label: 'Otro motivo' },
];

export const pedidosEntrantes = () =>
  api.get<{ orders?: PedidoEntrante[] }>('/orders/incoming').then((r) => r.orders ?? []);

export const aceptarEntrante = (p: Pick<PedidoEntrante, 'source' | 'id'>) =>
  api.post<PedidoAceptado>(`/orders/incoming/${p.source}/${p.id}/accept`, {});

export const rechazarEntrante = (p: Pick<PedidoEntrante, 'source' | 'id'>, reason: MotivoDeRechazo) =>
  api.post<void>(`/orders/incoming/${p.source}/${p.id}/reject`, { reason });
