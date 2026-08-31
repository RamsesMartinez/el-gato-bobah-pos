import { api } from './client';
import type { BoardOrder, Menu, OrderView, PaymentMethod, RankedOption } from '../types/pos';
import type { SessionUser } from '../stores/session';

export const posApi = {
  // Login multi-empresa: un solo identificador usuario@empresa. El backend separa el @ y
  // resuelve la empresa por su slug.
  login: (identifier: string, password: string) =>
    api.post<{ accessToken: string; user: SessionUser }>('/auth/login', { username: identifier, password }),
  pinSwitch: (userId: number, pin: string) =>
    api.post<{ accessToken: string; user: SessionUser }>('/auth/pin-switch', { userId, pin }),
  // Revoca el refresh token y borra la cookie en el server. Sin esto, "Salir" solo limpia
  // memoria y la sesión revive tras un reload (el arranque canjea la cookie que sobrevive).
  logout: () => api.post<void>('/auth/logout'),

  // Recuperación de contraseña. forgot: un solo identificador usuario@empresa; siempre 204
  // (anti-enumeración). reset: token del email.
  forgotPassword: (identifier: string) =>
    api.post<void>('/auth/forgot', { username: identifier }),
  resetPassword: (token: string, password: string) =>
    api.post<void>('/auth/reset', { token, password }),

  // Cuenta propia (cualquier empleado).
  changeOwnPassword: (currentPassword: string, newPassword: string) =>
    api.post<void>('/me/password', { currentPassword, newPassword }),
  setOwnPin: (pin: string) => api.post<void>('/me/pin', { pin }),

  // Agregar renglones a un pedido en curso: la libreta vuelve de la mesa con "pidieron dos más".
  // Se manda el DELTA, no el pedido completo — mandar la lista entera obligaría al servidor a
  // adivinar qué renglón es nuevo para no volver a descontar su stock.
  addOrderLines: (orderId: number, lines: CreateOrderBody['lines']) =>
    api.post<OrderView>(`/orders/${orderId}/lines`, { lines }),

  // Precios por plataforma: solo las EXCEPCIONES. Quitar una devuelve el producto al calculado.
  // El servidor valida que el producto y la plataforma sean de la empresa antes de escribir, así
  // que aquí van ids pelones.
  setPlatformPrice: (productId: number, platformId: number, price: number) =>
    api.put<{ productId: number; platformId: number; price: string }>(
      '/platform-prices/product', { productId, platformId, price }),
  removePlatformPrice: (productId: number, platformId: number) =>
    api.del<void>(`/platform-prices/product?productId=${productId}&platformId=${platformId}`),

  // Lo mismo para el cargo de un extra. El delta SÍ admite 0 —"sin cebolla" es un extra normal sin
  // costo—, a diferencia del precio de un producto.
  setPlatformOptionPrice: (optionId: number, platformId: number, priceDelta: number) =>
    api.put<{ optionId: number; platformId: number; priceDelta: string }>(
      '/platform-prices/modifier-option', { optionId, platformId, priceDelta }),
  removePlatformOptionPrice: (optionId: number, platformId: number) =>
    api.del<void>(`/platform-prices/modifier-option?optionId=${optionId}&platformId=${platformId}`),

  // Empresa (tenant). GET cualquiera; PATCH solo admin/gerente (backend aplica el 403).
  company: () => api.get<Company>('/company'),
  updateCompany: (name: string, slug: string) => api.patch<Company>('/company', { name, slug }),

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
  // Entregar. Son dos caminos porque son dos gestos distintos: "ya se llevó todo" es un tap sobre
  // la tarjeta, y "salieron 3 de 5 alitas" es sobre un renglón.
  deliverOrder: (id: number) => api.post<void>(`/orders/${id}/deliver`, {}),
  deliverLine: (id: number, lineId: number, qty: number) =>
    api.post<void>(`/orders/${id}/lines/${lineId}/deliver`, { qty }),

  // Ajustes de negocio. GET lo puede leer cualquier autenticado (el cobro lo necesita); el
  // PUT lo restringe el backend a admin/gerente.
  // ¿hay caja abierta? Para el aviso del POS (disponible a cualquier rol).
  cashStatus: () => api.get<{ open: boolean }>('/cash-status'),

  businessSettings: () => api.get<BusinessSettings>('/business-settings'),
  // Binario del logo. Lanza ApiError 404 cuando el negocio no subió ninguno: quien llama cae al
  // logo por default.
  ticketLogo: () => api.getRaw('/business-settings/ticket-logo'),
  // Configuración del ticket. Campos ausentes = no se tocan, así que esta pantalla no pisa el
  // costo de envío que edita la de Negocio.
  updateTicketSettings: (body: TicketSettingsInput) => api.put<BusinessSettings>('/business-settings', body),
  uploadTicketLogo: (file: File) => {
    const fd = new FormData();
    fd.append('file', file);
    return api.putForm<BusinessSettings>('/business-settings/ticket-logo', fd);
  },
  deleteTicketLogo: () => api.del<BusinessSettings>('/business-settings/ticket-logo'),
  updateBusinessSettings: (deliveryFee: number) =>
    api.put<BusinessSettings>('/business-settings', { deliveryFee }),
  // La zona decide de qué DÍA es una venta, un corte o un gasto. Va sola en su llamada porque
  // los campos ausentes no se tocan: guardarla no debe pisar el costo de envío ni el ticket.
  updateTimezone: (timezone: string) =>
    api.put<BusinessSettings>('/business-settings', { timezone }),
};

// El dinero viaja como string decimal exacto (ver types/pos.ts).
export interface BusinessSettings {
  deliveryFee: string;
  // Identidad que va en el encabezado del ticket. Los opcionales llegan como string vacío y no
  // como null: el ticket omite el renglón cuando está vacío.
  businessName: string;
  address: string;
  phone: string;
  headerNote: string;
  footerNote: string;
  // Si el POS imprime el ticket solo al cerrar una venta. Lo lee cualquier autenticado porque
  // quien tiene que obedecerlo es la caja, no el panel.
  autoPrintOnClose: boolean;
  // Nombre IANA de la zona del local. La base guarda instantes en UTC; esta zona es la que decide
  // de qué DÍA es cada venta, corte y gasto. El servidor rechaza un nombre que no exista.
  timezone: string;
  // Si el ticket lista los adicionales que no cuestan. Encendido por default: cocina los usa para
  // preparar y el cliente para reclamar; apagarlo solo acorta el papel.
  printFreeModifiers: boolean;
  // Si al mandar el pedido sale una comanda SIN precios para cocina. Apagado por default: donde la
  // cocina está pegada al mostrador sería papel que duplica lo que el cocinero ya ve.
  printKitchenTicket: boolean;
  // El binario NO viene aquí: se pide por su propio endpoint. hasLogo evita pedirlo cuando no hay,
  // y logoUpdatedAt sirve de versión para invalidar la copia en caché.
  hasLogo: boolean;
  logoUpdatedAt: string | null;
}

export interface Company {
  id: number;
  slug: string;
  name: string;
  isActive: boolean;
}

export type ModifierDefaults = Record<string, Record<string, RankedOption[]>>;

export interface CreateOrderBody {
  clientUuid: string;
  serviceType: string;
  customerName?: string;
  notes?: string;
  // Nombre con el que la pantalla ya bautizó la cuenta. El servidor lo sanea y resuelve los
  // choques del día, así que proponerlo no es decidirlo.
  folioName?: string;
  deliveryFee?: number; // solo aplica a domicilio; el server lo ignora si no
  // Con qué lista de precios se armó. El servidor la resuelve BAJO RLS y recalcula cada precio:
  // lo que va aquí es el id, nunca los precios.
  deliveryPlatformId?: number;
  lines: Array<{
    productId: number;
    qty: number;
    notes?: string;
    modifiers: Array<{ optionId: number; qty: number }>;
  }>;
  // pago dividido: una línea por método. El pedido queda pagado cuando la suma cubre el total.
  payments?: Array<{ methodId: number; amount: number; tip?: number }>;
}

// Lo editable de la configuración del ticket. Todo opcional: se manda solo lo que cambió.
export interface TicketSettingsInput {
  businessName?: string;
  address?: string;
  phone?: string;
  headerNote?: string;
  footerNote?: string;
  autoPrintOnClose?: boolean;
  printFreeModifiers?: boolean;
  printKitchenTicket?: boolean;
}
