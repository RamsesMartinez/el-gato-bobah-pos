import { describe, expect, it } from 'vitest';
import { porCobrar, resumenPorCobrar } from './porCobrar';
import type { BoardOrder } from '../../types/pos';

const orden = (o: Partial<BoardOrder>): BoardOrder => ({
  id: 1, number: 1, folioName: 'Tigre', status: 'abierta', serviceType: 'mostrador',
  customerName: null, total: '100', currency: 'MXN', paid: true,
  openedAt: '2026-08-31T18:00:00Z', lines: 1, linesDelivered: 0, ...o,
});

describe('lo que falta cobrar', () => {
  // El cuadro que cuesta dinero: entregado y sin cobrar. El cliente ya se fue con la comida.
  it('junta lo sin cobrar de los pedidos activos y de los entregados', () => {
    const activos = [orden({ id: 1, paid: true }), orden({ id: 2, paid: false, total: '150' })];
    const entregados = [orden({ id: 3, paid: false, total: '80', status: 'entregada' })];

    expect(porCobrar([...activos, ...entregados]).map((o) => o.id)).toEqual([2, 3]);
  });

  it('el resumen dice cuántos y cuánto', () => {
    const r = resumenPorCobrar([
      orden({ id: 2, paid: false, total: '150' }),
      orden({ id: 3, paid: false, total: '80.50' }),
      orden({ id: 4, paid: true, total: '999' }),
    ]);
    expect(r).toEqual({ cuantos: 2, monto: 230.5 });
  });

  // Un pedido cancelado no se cobra: contarlo mandaría al operador a perseguir dinero que nadie
  // debe, y a desconfiar del contador la próxima vez.
  it('lo cancelado no cuenta como por cobrar', () => {
    const r = resumenPorCobrar([orden({ id: 5, paid: false, total: '200', status: 'cancelada' })]);
    expect(r).toEqual({ cuantos: 0, monto: 0 });
  });

  it('sin nada pendiente el resumen queda en cero', () => {
    expect(resumenPorCobrar([orden({ paid: true })])).toEqual({ cuantos: 0, monto: 0 });
  });
});
