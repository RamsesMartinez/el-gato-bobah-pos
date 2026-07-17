import { api } from './client';

export interface MethodTotal {
  methodId: number;
  name: string;
  expected: number;
  declared: number;
  difference: number;
}
export interface CashSession {
  id: number;
  status: string;
  openingCash: number;
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
  amount: number;
  description: string | null;
}
export interface StockLevel {
  item_type: string;
  item_name: string;
  on_hand: number;
  min_stock: number | null;
  unit_code: string;
}
export interface StockMovement {
  id: number;
  item_type: string;
  item_name: string;
  movement_type: string;
  quantity: number;
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
      byDay: Array<{ business_date: string; orders: number; revenue: number }>;
      byMethod: Array<{ method: string; payments: number; total: number }>;
    }>(`/reports/sales${from ? `?from=${from}&to=${to}` : ''}`),
  reportMargins: () =>
    api.get<{ items: Array<{ product_name: string; qty: number; revenue: number; cost: number; margin: number }> }>(
      '/reports/margins?limit=50',
    ),
};
