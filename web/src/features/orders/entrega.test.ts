import { describe, expect, it } from 'vitest';
import { faltante, renglonesPendientes, resumenEntrega } from './entrega';
import type { OrderLine, OrderView } from '../../types/pos';

const linea = (l: Partial<OrderLine>): OrderLine => ({
  id: 1, productName: 'Alitas', quantity: '5', delivered: '0', cancelled: false,
  unitPrice: '200', lineTotal: '1000', ...l,
});

const pedido = (lines: OrderLine[]): OrderView => ({
  id: 1, number: 1, folioName: 'Tigre', status: 'abierta', serviceType: 'mostrador',
  customerName: null, subtotal: '0', deliveryFee: '0', total: '0', currency: 'MXN',
  paid: true, openedAt: '2026-08-31T18:00:00Z', lines,
});

describe('faltante', () => {
  it('resta lo entregado de lo pedido', () => {
    expect(faltante(linea({ quantity: '5', delivered: '3' }))).toBe(2);
  });

  it('es cero cuando ya salió todo', () => {
    expect(faltante(linea({ quantity: '5', delivered: '5' }))).toBe(0);
  });

  // Esa comida no se hizo: si contara como pendiente, el tablero pediría entregar algo que nadie
  // va a preparar y el pedido no podría cerrarse nunca.
  it('un renglón cancelado no debe nada', () => {
    expect(faltante(linea({ quantity: '5', delivered: '0', cancelled: true }))).toBe(0);
  });

  it('maneja fracciones', () => {
    expect(faltante(linea({ quantity: '1.5', delivered: '0.75' }))).toBe(0.75);
  });
});

describe('renglonesPendientes', () => {
  it('deja fuera lo ya entregado y lo cancelado', () => {
    const p = pedido([
      linea({ id: 1, quantity: '5', delivered: '5' }),
      linea({ id: 2, quantity: '2', delivered: '0' }),
      linea({ id: 3, quantity: '1', cancelled: true }),
      linea({ id: 4, quantity: '5', delivered: '3' }),
    ]);
    expect(renglonesPendientes(p).map((l) => l.id)).toEqual([2, 4]);
  });

  it('un pedido sin líneas no truena', () => {
    expect(renglonesPendientes({ ...pedido([]), lines: undefined })).toEqual([]);
  });
});

describe('resumenEntrega', () => {
  it('cuenta renglones completos, no piezas', () => {
    const p = pedido([
      linea({ id: 1, quantity: '5', delivered: '5' }),
      linea({ id: 2, quantity: '9', delivered: '8' }),
    ]);
    // 13 de 14 piezas, pero 1 de 2 productos: lo segundo es lo que el operador puede verificar
    // contra la charola.
    expect(resumenEntrega(p)).toEqual({ total: 2, entregados: 1, completo: false });
  });

  it('no cuenta los cancelados en el total', () => {
    const p = pedido([
      linea({ id: 1, quantity: '5', delivered: '5' }),
      linea({ id: 2, quantity: '1', cancelled: true }),
    ]);
    expect(resumenEntrega(p)).toEqual({ total: 1, entregados: 1, completo: true });
  });

  // Un pedido cancelado renglón a renglón no es un pedido entregado: nadie recibió nada.
  it('todo cancelado no es completo', () => {
    const p = pedido([linea({ id: 1, quantity: '1', cancelled: true })]);
    expect(resumenEntrega(p).completo).toBe(false);
  });

  it('un pedido sin líneas no es completo', () => {
    expect(resumenEntrega(pedido([])).completo).toBe(false);
  });
});
