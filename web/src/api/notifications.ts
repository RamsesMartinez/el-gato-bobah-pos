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

// Alertas de stock bajo de un artículo (spec 084, historia 3). Cantidades en la unidad base, en texto.
export type AlertItemType = 'ingrediente' | 'producto';

export interface LowStockConfig {
  enabled: boolean;
  onMin: boolean;
  onOut: boolean;
  banner: boolean;
  minStock: string | null;
  branchMinStock: string | null;
  coverageDays: number | null;
  asked: boolean;
  onHand: string;
  unitCode: string;
  branches: number;
  updatedAt: string;
}

export interface LowStockInput {
  enabled: boolean;
  onMin: boolean;
  onOut: boolean;
  banner: boolean;
  minStock: string | null;
  coverageDays: number | null;
  expectedUpdatedAt?: string;
}

export interface LowStockEffect {
  effect: { opens: NotificationKind[]; closes: NotificationKind[] };
  onHand: string;
  min: string | null;
  needsCount: boolean;
  updatedAt?: string;
}

export interface LowStockRow {
  itemType: AlertItemType;
  id: number;
  name: string;
  unitCode: string;
  enabled: boolean;
  minStock: string | null;
  onHand: string;
}

const ruta = (t: AlertItemType, id: number) => `/stock/${t === 'ingrediente' ? 'articles' : 'products'}/${id}/low-stock`;

export const lowStockApi = {
  get: (t: AlertItemType, id: number) => api.get<LowStockConfig>(ruta(t, id)),
  preview: (t: AlertItemType, id: number, body: LowStockInput) => api.put<LowStockEffect>(`${ruta(t, id)}?preview=1`, body),
  save: (t: AlertItemType, id: number, body: LowStockInput) => api.put<LowStockEffect>(ruta(t, id), body),
  asked: (id: number) => api.post<void>(`/stock/articles/${id}/low-stock/asked`),
  list: () => api.get<{ items?: LowStockRow[] }>('/stock/low-stock'),
  batch: (items: { itemType: AlertItemType; id: number; enabled: boolean; minStock: string | null }[], preview: boolean) =>
    api.post<{ items?: { itemType: AlertItemType; id: number; effect: LowStockEffect['effect']; onHand: string }[] }>(
      `/stock/low-stock/batch${preview ? '?preview=1' : ''}`, { items }),
};

export interface ShoppingLine {
  itemType: AlertItemType;
  id: number;
  name: string;
  unit: string;
  onHand: string;
  min: string | null;
  dailyUse: string | null;
  coverageDays: number;
  supplier: string;
  state: 'out' | 'low' | 'count';
  suggested: { packs: number; packName: string; qty: string } | null;
  why: string;
}

export const shoppingApi = {
  list: () => api.get<{ items?: ShoppingLine[] }>('/stock/shopping-list'),
  emailMe: () => api.post<{ sentTo: string }>('/stock/shopping-list/email'),
};

export interface RecipientPerson { userId: number; name: string; role: string; email: string; hasEmail: boolean; stock: boolean; notFound: boolean }
export interface RecipientExtra { email: string; stock: boolean; notFound: boolean }
export interface RecipientsView { morningAt: string; people: RecipientPerson[]; extras: RecipientExtra[] }

export const recipientsApi = {
  get: () => api.get<RecipientsView>('/settings/notification-recipients'),
  save: (v: RecipientsView) => api.put<void>('/settings/notification-recipients', v),
};
