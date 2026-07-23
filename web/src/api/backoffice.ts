import { api } from './client';
import type { PaymentMethod } from '../types/pos';

// Dinero/cantidades = string decimal exacto desde el backend (ver types/pos.ts).
export interface MethodTotal {
  methodId: number;
  name: string;
  expected: string;
  declared: string;
  difference: string;
  autoDeclare: boolean;
}
export interface CashMovement {
  id: number;
  kind: 'entrada' | 'salida';
  amount: string;
  concept: string;
  createdAt: string;
  userName: string;
  transferId: number | null; // no-null si el movimiento es una pierna de un traspaso entre cajas
  expenseId: number | null;  // no-null si es la salida de un gasto (se muestra en la sección Gastos)
}
// Gasto atribuido a un corte (sección "Gastos" del resumen).
export interface CashExpenseLine {
  id: number;
  category: string;
  supplier: string | null;
  paymentMethod: string | null;
  amount: string;
  currency: string;
  status: string;
}
// Descomposición jerárquica del corte: ingresos por método→concepto y egresos de efectivo.
export interface CorteBucket { concept: string; amount: string }
export interface CorteMethodBreakdown { method: string; total: string; items: CorteBucket[] }
export interface CorteBreakdown {
  ingresos: CorteMethodBreakdown[];
  ingresosTotal: string;
  egresos: CorteBucket[];
  egresosTotal: string;
}
// Caja física (registro). La primaria recibe las ventas del POS.
export interface CashRegister {
  id: number;
  name: string;
  isPrimary: boolean;
  isActive: boolean;
  openSessionId: number | null; // no-null si la caja tiene una sesión abierta
}
export interface CashSession {
  id: number;
  registerId: number;
  registerName: string;
  isPrimary: boolean;
  status: string;
  openingCash: string;
  currency: string;
  openedAt: string;
  netMovements: string;
  totals: MethodTotal[];
  movements: CashMovement[];
  expenses: CashExpenseLine[];
  breakdown: CorteBreakdown;
}
// Fila del histórico de cortes.
export interface CashSessionRow {
  id: number;
  registerName: string;
  status: string;
  openingCash: string;
  currency: string;
  openedAt: string;
  closedAt: string | null;
  openedByName: string;
  closedByName: string | null;
  totalDifference: string;
  notes: string | null;
}
// Detalle de un corte (totales GUARDADOS al cerrar + movimientos).
export interface CashSessionDetail {
  id: number;
  registerName: string;
  status: string;
  openingCash: string;
  currency: string;
  openedAt: string;
  closedAt: string | null;
  openedByName: string;
  closedByName: string | null;
  notes: string | null;
  totals: MethodTotal[];
  movements: CashMovement[];
  expenses: CashExpenseLine[];
  breakdown: CorteBreakdown;
}
export type FinancialGroup = 'operacional' | 'administrativo' | 'otro';
export type ExpenseStatus = 'pendiente' | 'pagada' | 'cancelada';

export interface ExpenseCategory {
  id: number;
  name: string;
  financialGroup: FinancialGroup;
  isActive: boolean;
}
export interface Supplier {
  id: number;
  name: string;
  phone: string | null;
  notes: string | null;
  isActive: boolean;
}
export interface Expense {
  id: number;
  expenseDate: string; // YYYY-MM-DD
  status: ExpenseStatus;
  category: string;
  financialGroup: FinancialGroup;
  supplier: string | null;
  amount: string;
  currency: string;
  description: string | null;
  paymentMethod: string | null;
  paidAt: string | null;
  createdBy: string | null;
}
export interface StockLevel {
  item_type: string;
  item_name: string;
  on_hand: string;
  min_stock: string | null;
  unit_code: string;
}
export interface StockMovement {
  id: number;
  item_type: string;
  item_name: string;
  movement_type: string;
  quantity: string;
  reason: string | null;
  created_at: string;
}

export const backofficeApi = {
  // Cajas (catálogo). `cashRegisters` = activas + estado de sesión; `allCashRegisters` = gestión.
  cashRegisters: () => api.get<{ items: CashRegister[] }>('/cash-registers'),
  allCashRegisters: () => api.get<{ items: CashRegister[] }>('/cash-registers/all'),
  createCashRegister: (name: string) => api.post<CashRegister>('/cash-registers', { name }),
  updateCashRegister: (id: number, b: { name: string; isActive: boolean }) =>
    api.patch<CashRegister>(`/cash-registers/${id}`, b),

  cashCurrent: (registerId: number) => api.get<CashSession | null>(`/cash-sessions/current?registerId=${registerId}`),
  cashOpen: (registerId: number, openingCash: number) => api.post<CashSession>('/cash-sessions', { registerId, openingCash }),
  cashClose: (registerId: number, declared: Record<string, number>, notes?: string) =>
    api.post<CashSession>('/cash-sessions/close', { registerId, declared, notes }),
  cashHistory: () => api.get<{ items: CashSessionRow[] }>('/cash-sessions'),
  cashSession: (id: number) => api.get<CashSessionDetail>(`/cash-sessions/${id}`),
  cashMovement: (registerId: number, kind: 'entrada' | 'salida', amount: number, concept: string) =>
    api.post<CashSession>('/cash-sessions/movements', { registerId, kind, amount, concept }),
  // Traspaso de efectivo entre dos cajas abiertas (genera salida en origen + entrada en destino).
  cashTransfer: (fromRegisterId: number, toRegisterId: number, amount: number, note?: string) =>
    api.post<{ id: number }>('/cash-sessions/transfer', { fromRegisterId, toRegisterId, amount, note }),
  // Config de negocio (admin/gerente): qué método se declara solo al cerrar caja.
  setPaymentMethodAutoDeclare: (id: number, autoDeclare: boolean) =>
    api.patch<PaymentMethod>(`/payment-methods/${id}`, { autoDeclare }),

  // Categorías de gasto
  expenseCategories: () => api.get<{ items: ExpenseCategory[] }>('/expense-categories'),
  createExpenseCategory: (b: { name: string; financialGroup: FinancialGroup }) =>
    api.post<ExpenseCategory>('/expense-categories', b),
  updateExpenseCategory: (id: number, b: { name: string; financialGroup: FinancialGroup; isActive: boolean }) =>
    api.patch<ExpenseCategory>(`/expense-categories/${id}`, b),

  // Proveedores
  suppliers: () => api.get<{ items: Supplier[] }>('/suppliers'),
  createSupplier: (b: { name: string; phone?: string; notes?: string }) =>
    api.post<Supplier>('/suppliers', b),
  updateSupplier: (id: number, b: { name: string; phone?: string; notes?: string; isActive: boolean }) =>
    api.patch<Supplier>(`/suppliers/${id}`, b),

  // Gastos (paginado)
  expenses: (params?: { status?: ExpenseStatus; page?: number; pageSize?: number }) => {
    const q = new URLSearchParams();
    if (params?.status) q.set('status', params.status);
    if (params?.page != null) q.set('page', String(params.page));
    if (params?.pageSize != null) q.set('pageSize', String(params.pageSize));
    const qs = q.toString();
    return api.get<{ items: Expense[]; total: number; page: number; pageSize: number }>(`/expenses${qs ? `?${qs}` : ''}`);
  },
  createExpense: (b: {
    categoryId: number; supplierId?: number; amount: number;
    description?: string; status: ExpenseStatus; methodId?: number; registerId?: number;
  }) => api.post<{ id: number }>('/expenses', b),
  payExpense: (id: number, methodId: number, registerId: number) => api.post<void>(`/expenses/${id}/pay`, { methodId, registerId }),
  cancelExpense: (id: number, reason: string) => api.post<void>(`/expenses/${id}/cancel`, { reason }),

  stockLevels: () => api.get<{ items: StockLevel[] }>('/stock/levels'),
  stockMovements: () => api.get<{ items: StockMovement[] }>('/stock/movements'),
  createMovement: (b: {
    itemType: string;
    ingredientId?: number;
    productId?: number;
    movementType: string;
    quantity: number;
    reason?: string;
  }) => api.post<void>('/stock/movements', b),

  reportSales: (from?: string, to?: string) =>
    api.get<{
      byDay: Array<{ business_date: string; orders: number; revenue: string }>;
      byMethod: Array<{ method: string; payments: number; total: string }>;
    }>(`/reports/sales${from ? `?from=${from}&to=${to}` : ''}`),
  reportMargins: () =>
    api.get<{ items: Array<{ product_name: string; qty: string; revenue: string; cost: string; margin: string }> }>(
      '/reports/margins?limit=50',
    ),
  // Propinas (pass-through, para repartir): por empleado que cobró y por día.
  reportTips: (from?: string, to?: string) =>
    api.get<{
      byEmployee: Array<{ employee: string; payments: number; tips: string }>;
      byDay: Array<{ business_date: string; tips: string }>;
    }>(`/reports/tips${from ? `?from=${from}&to=${to}` : ''}`),
};
