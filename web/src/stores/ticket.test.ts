import { describe, it, expect } from 'vitest';
import { lineTotal, ticketTotal, ticketCount, useTicketStore } from './ticket';
import type { TicketLine } from '../types/pos';

function activeLines() {
  const s = useTicketStore.getState();
  return (s.tabs.find((t) => t.id === s.activeId) ?? s.tabs[0]).lines;
}

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

describe('addLine — merge de productos directos', () => {
  it('un producto directo tocado 2 veces suma cantidad, no duplica la línea', () => {
    const s = useTicketStore.getState();
    s.clearActive();
    s.addLine({ productId: 7, name: 'Café', unitPrice: 30, qty: 1, modifiers: [] });
    s.addLine({ productId: 7, name: 'Café', unitPrice: 30, qty: 1, modifiers: [] });
    const lines = activeLines();
    expect(lines).toHaveLength(1);
    expect(lines[0].qty).toBe(2);
  });

  it('con modificadores NO fusiona: cada configuración es su propia línea', () => {
    const s = useTicketStore.getState();
    s.clearActive();
    s.addLine({ productId: 8, name: 'Crepa', unitPrice: 50, qty: 1,
      modifiers: [{ optionId: 1, groupId: 1, name: 'Nutella', priceDelta: 10, qty: 1 }] });
    s.addLine({ productId: 8, name: 'Crepa', unitPrice: 50, qty: 1,
      modifiers: [{ optionId: 2, groupId: 1, name: 'Cajeta', priceDelta: 10, qty: 1 }] });
    expect(activeLines()).toHaveLength(2);
  });

  it('con nota NO fusiona (una nota es específica de esa línea)', () => {
    const s = useTicketStore.getState();
    s.clearActive();
    s.addLine({ productId: 9, name: 'Jugo', unitPrice: 25, qty: 1, modifiers: [], notes: 'sin hielo' });
    s.addLine({ productId: 9, name: 'Jugo', unitPrice: 25, qty: 1, modifiers: [], notes: 'sin hielo' });
    expect(activeLines()).toHaveLength(2);
  });

  it('fusiona con la línea directa correcta aunque haya otro producto en medio', () => {
    const s = useTicketStore.getState();
    s.clearActive();
    s.addLine({ productId: 1, name: 'A', unitPrice: 10, qty: 1, modifiers: [] });
    s.addLine({ productId: 2, name: 'B', unitPrice: 10, qty: 1, modifiers: [] });
    s.addLine({ productId: 1, name: 'A', unitPrice: 10, qty: 1, modifiers: [] });
    const lines = activeLines();
    expect(lines).toHaveLength(2);
    expect(lines.find((l) => l.productId === 1)?.qty).toBe(2);
  });
});

test('cada cuenta lleva su propia lista de precios y una nueva arranca en mostrador', () => {
  const st = useTicketStore.getState();
  st.descartarTodo();
  expect(useTicketStore.getState().tabs[0].platformId).toBeNull();

  useTicketStore.getState().setPlatform(5);
  expect(useTicketStore.getState().tabs[0].platformId).toBe(5);

  // Una cuenta nueva no hereda la plataforma: sería la forma de cobrar precio de Uber en
  // mostrador por inercia.
  useTicketStore.getState().newTab();
  const activa = useTicketStore.getState().tabs.find((t) => t.id === useTicketStore.getState().activeId);
  expect(activa?.platformId).toBeNull();

  // Y la anterior conserva la suya: se pueden tener las dos abiertas a la vez.
  expect(useTicketStore.getState().tabs[0].platformId).toBe(5);
});
