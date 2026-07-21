import { api } from './client';

// Dinero/cantidades = string decimal exacto desde el backend (ver types/pos.ts).
export interface MethodTotal {
  methodId: number;
  name: string;
  expected: string;
  declared: string;
  difference: string;
}
export interface CashSession {
  id: number;
  status: string;
  openingCash: string;
  currency: string;
  openedAt: string;
  totals: MethodTotal[];
}
export interface ExpenseCategory {
  id: number;
  name: string;
  financial_group: string;
}
export interface ExpenseRow {
  id: number;
  expense_date: string;
  category: string;
  supplier: string | null;
  amount: string;
  currency: string;
  description: string | null;
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
  cashClose: (declared: Record<string, number>) =>
    api.post<CashSession>('/cash-sessions/close', { declared }),

  expenseCategories: () => api.get<{ items: ExpenseCategory[] }>('/expense-categories'),
  expenses: () => api.get<{ items: ExpenseRow[] }>('/expenses'),
  createExpense: (b: { categoryId: number; amount: number; description?: string }) =>
    api.post<{ id: number }>('/expenses', b),

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
