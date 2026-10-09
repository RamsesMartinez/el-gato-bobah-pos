import type { AccountItem, AccountState } from '../types/pos';

// Cómo se dice y de qué color va cada estado de una cuenta viva. El estado lo decide el servidor
// (domain.AccountState); aquí solo se nombra. Vive en `domain` porque lo leen dos pantallas —Vender
// y el cierre de caja— y una pantalla no importa de otra: dos copias ya se estaban desalineando.
export const ESTADO: Record<AccountState, { texto: string; color: string }> = {
  capturing: { texto: 'Capturando', color: 'gray' },
  in_kitchen: { texto: 'En cocina', color: 'blue' },
  paid_in_kitchen: { texto: 'Pagada · en cocina', color: 'green' },
  partly_paid: { texto: 'Pago parcial', color: 'orange' },
  delivered_owes: { texto: 'Entregada · debe', color: 'red' },
  // Ya entregada y saldada (o cancelada); lo vivo es lo «Nuevo» que se está capturando.
  closed_with_new: { texto: 'Cerrada · nuevo', color: 'gray' },
};

export function nombreDeCuenta(c: Pick<AccountItem, 'folioName' | 'customerName'>): string {
  return c.folioName || c.customerName || 'Cuenta nueva';
}
