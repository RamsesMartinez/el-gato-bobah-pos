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
  search: (q: string) => api.get<{ items: OutItem[] }>(`/stock/out-reports/search?q=${encodeURIComponent(q)}`),
  suggestions: () => api.get<OutSuggestions>('/stock/out-reports/suggestions'),
  report: (item: OutItem, clientKey: string) =>
    api.post<OutReportResult>('/stock/out-reports', { itemType: item.itemType, id: item.id, clientKey }),
  undo: (id: number) => api.post<void>(`/stock/out-reports/${id}/undo`),
  notFound: (text: string, clientKey: string) =>
    api.post<{ id: number; notificationId: number }>('/stock/not-found', { text, clientKey }),
};
