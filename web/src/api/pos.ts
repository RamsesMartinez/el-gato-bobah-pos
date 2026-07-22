import { api } from './client';
import type { BoardOrder, Menu, OrderView, PaymentMethod, RankedOption } from '../types/pos';
import type { SessionUser } from '../stores/session';

export const posApi = {
  login: (username: string, password: string) =>
    api.post<{ accessToken: string; user: SessionUser }>('/auth/login', { username, password }),
  pinSwitch: (userId: number, pin: string) =>
    api.post<{ accessToken: string; user: SessionUser }>('/auth/pin-switch', { userId, pin }),
  // Revoca el refresh token y borra la cookie en el server. Sin esto, "Salir" solo limpia
  // memoria y la sesión revive tras un reload (el arranque canjea la cookie que sobrevive).
  logout: () => api.post<void>('/auth/logout'),

  menu: () => api.get<Menu>('/pos/menu'),
  // IDs de producto más vendidos (read model aparte, refresca cada pocos minutos).
  popular: () => api.get<{ items: number[] }>('/pos/popular'),
  // producto → grupo → [optionId rankeadas] por probabilidad contextual. Claves string (JSON).
  modifierDefaults: () => api.get<ModifierDefaults>('/pos/modifier-defaults'),
  paymentMethods: () => api.get<{ items: PaymentMethod[] }>('/payment-methods'),

  createOrder: (body: CreateOrderBody) => api.post<OrderView>('/orders', body),
  activeOrders: () => api.get<{ items: BoardOrder[] }>('/orders'),
  order: (id: number) => api.get<OrderView>(`/orders/${id}`),
  setOrderStatus: (id: number, status: string) =>
    api.post<void>(`/orders/${id}/status`, { status }),
  cancelOrder: (id: number, reason: string) =>
    api.post<void>(`/orders/${id}/cancel`, { reason }),
  // Entregadas del día + reembolso (solo admin/gerente; el backend aplica el 403).
  deliveredOrders: () => api.get<{ items: BoardOrder[] }>('/orders/delivered'),
  refundOrder: (id: number, reason: string) =>
    api.post<void>(`/orders/${id}/refund`, { reason }),

  // Ajustes de negocio. GET lo puede leer cualquier autenticado (el cobro lo necesita); el
  // PUT lo restringe el backend a admin/gerente.
  // ¿hay caja abierta? Para el aviso del POS (disponible a cualquier rol).
  cashStatus: () => api.get<{ open: boolean }>('/cash-status'),

  businessSettings: () => api.get<BusinessSettings>('/business-settings'),
  updateBusinessSettings: (deliveryFee: number) =>
    api.put<BusinessSettings>('/business-settings', { deliveryFee }),
};

// El dinero viaja como string decimal exacto (ver types/pos.ts).
export interface BusinessSettings {
  deliveryFee: string;
}

export type ModifierDefaults = Record<string, Record<string, RankedOption[]>>;

export interface CreateOrderBody {
  clientUuid: string;
  serviceType: string;
  customerName?: string;
  notes?: string;
  deliveryFee?: number; // solo aplica a domicilio; el server lo ignora si no
  lines: Array<{
    productId: number;
    qty: number;
    notes?: string;
    modifiers: Array<{ optionId: number; qty: number }>;
  }>;
  payment?: { methodId: number; amount: number; tip?: number };
}
