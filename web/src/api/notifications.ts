import { api } from './client';

// Avisos de inventario (spec 084): la campana, lo leído por persona y cerrar un aviso a mano.

export type NotificationKind = 'stock_low' | 'stock_out' | 'not_found';

export interface NotificationView {
  id: number;
  kind: NotificationKind;
  status: 'open' | 'resolved' | 'dismissed';
  itemType?: 'ingrediente' | 'producto';
  itemId?: number;
  itemName?: string;
  unitCode?: string;
  queryText?: string;
  branchName: string;
  onHandSeen: string | null;
  minSeen: string | null;
  needsCount: boolean;
  openedAt: string;
  openedById: number | null;
  openedByName?: string;
  resolvedAt: string | null;
  resolvedByName?: string;
  resolvedReason?: string;
  read: boolean;
  banner: boolean;
  confirmations: number;
}

export type ResolveAction = 'arrived' | 'dismiss' | 'created' | 'alias';

export const notificationsApi = {
  // La lista llega en `items`; se declara opcional para que el compilador obligue a la guarda
  // (AGENTS.md: un arreglo nil sale como null y tumba la pantalla sin un error).
  list: (status: 'open' | 'closed') => api.get<{ items?: NotificationView[] }>(`/notifications?status=${status}&limit=50`),
  unread: () => api.get<{ unread: number }>('/notifications/unread-count'),
  read: (ids: number[]) => api.post<void>('/notifications/read', { ids }),
  bannerSeen: (id: number) => api.post<void>(`/notifications/${id}/banner-seen`),
  resolve: (id: number, action: ResolveAction, ingredientId?: number) =>
    api.post<void>(`/notifications/${id}/resolve`, ingredientId ? { action, ingredientId } : { action }),
};

// Qué dice cada motivo de cierre, para quien opera.
export const REASON_WORDS: Record<string, string> = {
  above_min: 'Volvió sobre el mínimo',
  restocked: 'Volvió a haber',
  counted: 'Se contó',
  arrived: 'Llegó',
  undone: 'Se deshizo el aviso',
  alert_off: 'Se apagó la alerta',
  archived: 'Se archivó el artículo',
  merged: 'Se unió a otro insumo',
  unit_changed: 'Cambió en qué se mide',
  dismissed: 'Descartado',
  created: 'Se dio de alta',
  alias: 'Era otro nombre de un insumo',
};
