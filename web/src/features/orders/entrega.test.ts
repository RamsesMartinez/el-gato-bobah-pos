import { describe, expect, it } from 'vitest';
import { entregados, faltante, pendientes } from './entrega';
import type { BoardLine, BoardOrder } from '../../types/pos';

const linea = (l: Partial<BoardLine>): BoardLine => ({
  id: 1, name: 'Alitas', qty: '5', delivered: '0', ...l,
});

const pedido = (lines: BoardLine[]): BoardOrder => ({
  id: 1, number: 1, folioName: 'Tigre', status: 'abierta', serviceType: 'mostrador',
  deliveryPlatformId: null, customerName: null, total: '0', outstanding: '0',
  currency: 'MXN', paid: true,
  openedAt: '2026-08-31T18:00:00Z', lines,
});

describe('faltante', () => {
  it('resta lo entregado de lo pedido', () => {
    expect(faltante(linea({ qty: '5', delivered: '3' }))).toBe(2);
  });
  it('es cero cuando ya salió todo', () => {
    expect(faltante(linea({ qty: '5', delivered: '5' }))).toBe(0);
  });
  it('maneja fracciones', () => {
    expect(faltante(linea({ qty: '1.5', delivered: '0.75' }))).toBe(0.75);
  });
});

describe('pendientes', () => {
  it('deja fuera lo ya entregado', () => {
    const p = pedido([
      linea({ id: 1, qty: '5', delivered: '5' }),
      linea({ id: 2, qty: '2', delivered: '0' }),
      linea({ id: 3, qty: '5', delivered: '3' }),
    ]);
    expect(pendientes(p).map((l) => l.id)).toEqual([2, 3]);
  });
});

describe('entregados', () => {
  it('cuenta renglones completos, no piezas', () => {
    // 13 de 14 piezas, pero 1 de 2 productos: lo segundo es lo que se verifica contra la charola.
    const p = pedido([
      linea({ id: 1, qty: '5', delivered: '5' }),
      linea({ id: 2, qty: '9', delivered: '8' }),
    ]);
    expect(entregados(p)).toBe(1);
  });
  it('un pedido sin renglones no tiene nada entregado', () => {
    expect(entregados(pedido([]))).toBe(0);
  });
});
