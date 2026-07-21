import { describe, it, expect } from 'vitest';
import { buildReceiptHtml } from './printReceipt';
import type { OrderView } from '../types/pos';

const baseOrder: OrderView = {
  id: 1,
  number: 42,
  status: 'abierta',
  serviceType: 'mostrador',
  customerName: null,
  subtotal: '5000',
  total: '5000',
  currency: 'MXN',
  paid: false,
  openedAt: '2026-07-19T12:00:00Z',
  lines: [],
};

describe('buildReceiptHtml', () => {
  it('escapes attacker-controlled strings so they cannot inject markup/script', () => {
    const order: OrderView = {
      ...baseOrder,
      customerName: '<img src=x onerror=alert(1)>',
      lines: [
        {
          productName: '<script>alert(2)</script>',
          quantity: '1',
          unitPrice: '5000',
          lineTotal: '5000',
          modifiers: [{ name: '<b>extra</b>', quantity: 1, priceDelta: '0' }],
        },
      ],
    };
    const html = buildReceiptHtml(order);

    // No live markup from user data
    expect(html).not.toContain('<img src=x');
    expect(html).not.toContain('<script>alert(2)');
    expect(html).not.toContain('<b>extra</b>');
    // Escaped equivalents are present
    expect(html).toContain('&lt;img src=x');
    expect(html).toContain('&lt;script&gt;alert(2)');
  });

  it('renders normal order data verbatim (no double-escaping of safe text)', () => {
    const order: OrderView = {
      ...baseOrder,
      customerName: 'María',
      lines: [{ productName: 'Boba fresa', quantity: '2', unitPrice: '2500', lineTotal: '5000', modifiers: [] }],
    };
    const html = buildReceiptHtml(order);
    expect(html).toContain('María');
    expect(html).toContain('Boba fresa');
    expect(html).toContain('Pedido #42');
  });
});
