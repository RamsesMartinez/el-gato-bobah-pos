import type { AccountItem } from '../../../types/pos';

// Una cuenta viva como la manda GET /pos/accounts, para los tests.
export function cuenta(over: Partial<AccountItem> & { key: string }): AccountItem {
  return {
    kind: 'order', draftId: null, orderId: 1, number: 1, folioName: 'Khao Manee', state: 'in_kitchen',
    group: 'in_kitchen', kitchenReady: false, platformId: null, serviceType: 'mostrador', customerName: null,
    openedAt: '2026-10-08T15:00:00Z', updatedAt: '2026-10-08T15:00:00Z', businessDate: '2026-10-08',
    total: '194.00', paid: '0.00', outstanding: '194.00', lineCount: 3, pendingDraftId: null,
    pendingCount: 0, closedWithPending: false, ...over,
  };
}
