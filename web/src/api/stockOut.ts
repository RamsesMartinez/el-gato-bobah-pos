import { api } from './client';

// «Se acabó» y «No lo encuentro» (spec 084). Nunca lleva cantidad: el servidor deja la existencia en 0
// y rechaza con 400 un cuerpo que la traiga.

export type OutItemType = 'ingrediente' | 'producto';

export interface OutItem {
  itemType: OutItemType;
  id: number;
  name: string;
  unit?: string;
  openOut?: boolean;
  times?: number;
}

export interface OutReportRow {
  id: number;
  kind: 'report' | 'confirmation' | 'not_found';
  name: string;
  reportedAt: string;
  reportedById: number;
  reportedBy: string;
  undoneAt: string | null;
  moved: boolean;
  open: boolean;
  // Dónde se reportó (spec 094). Opcionales: la API anterior no los trae.
  warehouseId?: number;
  warehouseName?: string;
}

export interface OutSuggestions {
  frequent: OutItem[];
  today: OutReportRow[];
  undoSeconds: number;
}

export interface OutReportResult {
  id: number;
  kind: 'report' | 'confirmation';
  notificationId: number;
  delta: string | null;
  undoUntil: string;
  confirmations: number;
}

export const stockOutApi = {
  // «Ya reportado» es del almacén pedido (spec 094); sin warehouseId, el de venta.
  search: (q: string, warehouseId?: number) => api.get<{ items: OutItem[] }>(
    `/stock/out-reports/search?q=${encodeURIComponent(q)}${warehouseId === undefined ? '' : `&warehouseId=${warehouseId}`}`),
  suggestions: () => api.get<OutSuggestions>('/stock/out-reports/suggestions'),
  // Sin warehouseId es el almacén de venta (spec 094).
  report: (item: OutItem, clientKey: string, warehouseId?: number) =>
    api.post<OutReportResult>('/stock/out-reports', warehouseId === undefined
      ? { itemType: item.itemType, id: item.id, clientKey }
      : { itemType: item.itemType, id: item.id, clientKey, warehouseId }),
  undo: (id: number) => api.post<void>(`/stock/out-reports/${id}/undo`),
  notFound: (text: string, clientKey: string) =>
    api.post<{ id: number; notificationId: number }>('/stock/not-found', { text, clientKey }),
};
