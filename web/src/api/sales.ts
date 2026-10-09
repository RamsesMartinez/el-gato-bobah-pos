import { api } from './client';

// Pantalla de Ventas: el análisis de lo que ya pasó, distinto del tablero de pedidos.
//
// Son DOS llamadas y no una que traiga lista y resumen juntos. La razón es el gesto más repetido de
// la pantalla: paginar. Cambiar de página no cambia el resumen, y con una sola respuesta cada tap
// del paginador volvería a agregar todo el rango para tirar el resultado. Separadas, la llave del
// resumen no lleva la página y el caché la conserva.

// Dinero como string decimal exacto, igual que en el resto del sistema: pasarlo por float redondea
// distinto que el servidor y el total de pantalla deja de cuadrar con el cobrado.
export interface SaleRow {
  id: number;
  dailyNumber: number;
  // Nombre con el que se cantó el pedido. Es como el cliente pide su ticket para facturar.
  folioName: string;
  date: string;
  openedAt: string;
  completedAt: string | null;
  status: string;
  serviceType: string;
  customer: string;
  total: string;
  // Lo descontado en este pedido, "0" cuando no hubo. Es lo único que explica por qué un pedido
  // cobró menos de lo que vendió.
  discount: string;
  // Quién lo aplicó, vacío si no hubo descuento. Es lo que hace auditable un descuento que nadie
  // tuvo que autorizar.
  discountBy: string;
  deliveryFee: string;
  refund: string;
  tips: string;
  platform: string;
  // El folio con el que la plataforma nombró al pedido. Vacío = sin capturar, que es lo que el
  // filtro de pendientes lista.
  platformOrderRef: string;
  openedBy: string;
  methods: string;
  // Lo cobrado. Con `total` dice cuánto falta sin abrir el pedido (spec 029).
  paid: string;
  // Cuándo fue la última devolución; null si no hubo. Es lo que deja reconocer una devolución de
  // otro mes en la lista.
  lastRefundAt: string | null;
}

export interface SalesRange { from: string; to: string }

export interface SalesPage {
  range: SalesRange;
  items: SaleRow[];
  total: number;
}

export interface MethodTotals {
  methodId: number;
  method: string;
  payments: number;
  // Cobrado en el periodo menos lo devuelto en el periodo: cada cobro cuenta el día en que se cobró
  // y cada devolución el día en que se devolvió (spec 031). Puede ser negativo.
  total: string;
  tips: string;
  // Lo devuelto de la venta por este medio en el periodo, YA restado de `total`.
  refunds?: string;
  // Propina devuelta por este medio en el periodo. No está en `total` ni en `refunds`.
  tipRefunds?: string;
}

export interface ConceptCount { count: number; amount: string }

// Cada campo declara qué incluye, y la separación no es estética:
//   - total: lo COBRADO en el periodo menos lo devuelto en el periodo (spec 029, decisión del dueño):
//     cada cobro en su día y cada devolución en el suyo. Es la suma de `byMethod[].total`.
//   - count y average: cifras de VENTA (pedidos no cancelados y su ticket promedio), no de dinero.
//   - pending: lo que falta por cobrar de los pedidos del periodo. NO está en total.
//   - tips: dinero del personal que pasa por la caja. NO está dentro de total.
//   - deliveryFees: ya está DENTRO de total; viaja aparte solo como referencia.
export interface SalesSummary {
  range: SalesRange;
  count: number;
  total: string;
  average: string;
  tips: string;
  deliveryFees: string;
  cancelled: ConceptCount;
  // Devoluciones HECHAS en el periodo, las mismas que `byMethod` ya restó (no el estado de los pedidos).
  refunded: ConceptCount;
  byMethod: MethodTotals[];
  cancelledLines: ConceptCount;
  pending: ConceptCount;
}

export type SalesPreset = 'hoy' | 'ayer' | 'semana' | 'mes' | 'rango';
export type SalesSort = 'fecha' | 'folio' | 'total' | 'estado' | 'tipo';

export interface SalesQuery {
  preset?: SalesPreset;
  from?: string;
  to?: string;
  status?: string;
  serviceType?: string;
  sort?: SalesSort;
  dir?: 'asc' | 'desc';
  page?: number;
  pageSize?: number;
  // 'pendiente' acota a los pedidos de plataforma SIN folio. Cualquier otro valor lo rechaza el
  // servidor con un 400 a propósito: un filtro que cae a "todos" muestra un conjunto que nadie pidió.
  folioPlataforma?: 'pendiente';
  // El folio EXACTO pegado del documento de pago. Busca solo el folio de plataforma, nunca el
  // número ni el nombre del turno.
  folio?: string;
}

// Los vacíos NO viajan: el servidor aplica el default solo al parámetro ausente, y mandar
// `status=` sería mandarle un estado presente y desconocido, que rechaza a propósito.
function qs(q: SalesQuery): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) {
    if (v !== undefined && v !== '') p.set(k, String(v));
  }
  return p.toString();
}

export const salesApi = {
  list: (q: SalesQuery = {}) => api.get<SalesPage>(`/sales?${qs(q)}`),
  // El resumen no lleva página ni orden: no cambian con ellos, y meterlos en la llave haría que se
  // vuelva a pedir en cada tap del paginador.
  summary: (q: SalesQuery = {}) =>
    // El filtro de pendientes y la búsqueda SÍ viajan al resumen: si la lista y el resumen no
    // describen el mismo conjunto, quien lee la pantalla no tiene forma de saber cuál miente.
    api.get<SalesSummary>(`/sales/summary?${qs({
      preset: q.preset, from: q.from, to: q.to, serviceType: q.serviceType,
      folioPlataforma: q.folioPlataforma, folio: q.folio,
    })}`),
};
