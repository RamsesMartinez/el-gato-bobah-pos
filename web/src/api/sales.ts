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
  date: string;
  openedAt: string;
  completedAt: string | null;
  status: string;
  serviceType: string;
  customer: string;
  total: string;
  deliveryFee: string;
  refund: string;
  tips: string;
  platform: string;
  openedBy: string;
  methods: string;
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
  total: string;
  tips: string;
}

export interface ConceptCount { count: number; amount: string }

// Cada campo declara qué incluye, y la separación no es estética:
//   - total: ingreso REAL. NO incluye canceladas ni reembolsadas.
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
  refunded: ConceptCount;
  byMethod: MethodTotals[];
  cancelledLines: ConceptCount;
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
    api.get<SalesSummary>(`/sales/summary?${qs({ preset: q.preset, from: q.from, to: q.to, serviceType: q.serviceType })}`),
};
