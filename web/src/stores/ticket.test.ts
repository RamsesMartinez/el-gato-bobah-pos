import { describe, it, expect } from 'vitest';
import { lineTotal, ticketTotal, ticketCount } from './ticket';
import type { TicketLine } from '../types/pos';

function line(over: Partial<TicketLine>): TicketLine {
  return { lineId: 'x', productId: 1, name: 'P', unitPrice: 0, qty: 1, modifiers: [], ...over };
}

describe('ticket money math', () => {
  it('suma modificadores por unidad × cantidad', () => {
    const l = line({
      unitPrice: 45,
      qty: 2,
      modifiers: [
        { optionId: 1, groupId: 1, name: 'Perlas', priceDelta: 20, qty: 1 },
        { optionId: 2, groupId: 1, name: 'Litchi', priceDelta: 20, qty: 1 },
      ],
    });
    expect(lineTotal(l)).toBe(170); // (45+20+20)*2
  });

  it('total y conteo del ticket', () => {
    const lines = [line({ unitPrice: 30, qty: 1 }), line({ lineId: 'y', unitPrice: 12, qty: 3 })];
    expect(ticketTotal(lines)).toBe(66); // 30 + 36
    expect(ticketCount(lines)).toBe(4);
  });

  it('redondea a centavos', () => {
    const l = line({ unitPrice: 10.1, qty: 3 });
    expect(lineTotal(l)).toBe(30.3);
  });
});
