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
}
export interface CashSession {
  id: number;
  status: string;
  openingCash: string;
  currency: string;
  openedAt: string;
  netMovements: string;
  totals: MethodTotal[];
  movements: CashMovement[];
}
// Fila del histórico de cortes.
export interface CashSessionRow {
  id: number;
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
  cashCurrent: () => api.get<CashSession | null>('/cash-sessions/current'),
  cashOpen: (openingCash: number) => api.post<CashSession>('/cash-sessions', { openingCash }),
  cashClose: (declared: Record<string, number>, notes?: string) =>
    api.post<CashSession>('/cash-sessions/close', { declared, notes }),
  cashHistory: () => api.get<{ items: CashSessionRow[] }>('/cash-sessions'),
  cashSession: (id: number) => api.get<CashSessionDetail>(`/cash-sessions/${id}`),
  cashMovement: (kind: 'entrada' | 'salida', amount: number, concept: string) =>
    api.post<CashSession>('/cash-sessions/movements', { kind, amount, concept }),
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
    description?: string; status: ExpenseStatus; methodId?: number;
  }) => api.post<{ id: number }>('/expenses', b),
  payExpense: (id: number, methodId: number) => api.post<void>(`/expenses/${id}/pay`, { methodId }),
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
};
