import { api } from './client';

export interface AuditEvent {
  id: number;
  occurredAt: string;
  action: string;
  actorKind: 'user' | 'platform' | 'system';
  actorName: string;
  ip: string;
  device?: Record<string, string>;
  deviceKey: string;
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
  orderId?: number;
  // orderLabel: el pedido como lo nombra Ventas («Mesa 4 · #12»).
  orderLabel?: string;
}

export type AuditGroup = 'money' | 'cancel' | 'price' | 'cash' | 'attendance';

export interface AuditPage {
  items: AuditEvent[];
  total: number;
  countsByGroup: Record<AuditGroup, number>;
  // Opcional para que el compilador obligue a la guarda: un servidor viejo no lo manda.
  deviceKeys?: string[];
}

export interface AuditQuery {
  from: string;
  to: string;
  actorUserId?: number;
  group?: AuditGroup;
  action?: string;
  deviceKey?: string;
  page: number;
  size: 25 | 50;
}

export const auditApi = {
  orderHistory: (orderId: number) => api.get<AuditEvent[]>(`/orders/${orderId}/history`),
  list: (q: AuditQuery) => {
    const p = new URLSearchParams({ from: q.from, to: q.to, page: String(q.page), size: String(q.size) });
    if (q.actorUserId) p.set('actorUserId', String(q.actorUserId));
    if (q.group) p.set('group', q.group);
    if (q.action) p.set('action', q.action);
    if (q.deviceKey) p.set('deviceKey', q.deviceKey);
    return api.get<AuditPage>(`/admin/audit?${p.toString()}`);
  },
  actors: () => api.get<{ id: number; name: string }[]>('/admin/audit/actors'),
};
