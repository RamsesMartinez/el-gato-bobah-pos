import { describe, expect, it } from 'vitest';
import { porCobrar, resumenPorCobrar } from './porCobrar';
import type { BoardOrder } from '../../types/pos';

const orden = (o: Partial<BoardOrder>): BoardOrder => ({
  id: 1, number: 1, folioName: 'Tigre', status: 'abierta', serviceType: 'mostrador',
  deliveryPlatformId: null, customerName: null, total: '100', outstanding: '0',
  currency: 'MXN', paid: true, enPreparacion: false, renglones: 1,
  openedAt: '2026-08-31T18:00:00Z', lines: [], ...o,
});

describe('lo que falta cobrar', () => {
  // El cuadro que cuesta dinero: entregado y sin cobrar. El cliente ya se fue con la comida.
  it('junta lo sin cobrar de los pedidos activos y de los entregados', () => {
    const activos = [
      orden({ id: 1, paid: true }),
      orden({ id: 2, paid: false, enPreparacion: true, renglones: 1, total: '150', outstanding: '150' }),
    ];
    const entregados = [
      orden({ id: 3, paid: false, enPreparacion: true, renglones: 1, total: '80', outstanding: '80', status: 'entregada' }),
    ];

    expect(porCobrar([...activos, ...entregados]).map((o) => o.id)).toEqual([2, 3]);
  });

  it('el resumen dice cuántos y cuánto', () => {
    const r = resumenPorCobrar([
      orden({ id: 2, paid: false, enPreparacion: true, renglones: 1, total: '150', outstanding: '150' }),
      orden({ id: 3, paid: false, enPreparacion: true, renglones: 1, total: '80.50', outstanding: '80.50' }),
      orden({ id: 4, paid: true, enPreparacion: false, renglones: 1, total: '999' }),
    ]);
    expect(r).toEqual({ cuantos: 2, monto: 230.5 });
  });

  // Un pedido ABONADO debe menos de lo que costó. Sumar el total mandaría al operador a cobrar
  // dos veces la parte que el cliente ya dejó, y el aviso del tablero reportaría de más.
  it('un pedido abonado cuenta solo por lo que falta', () => {
    const r = resumenPorCobrar([
      orden({ id: 6, paid: false, enPreparacion: true, renglones: 1, total: '250', outstanding: '150' }),
    ]);
    expect(r).toEqual({ cuantos: 1, monto: 150 });
  });

  // Un pedido cancelado no se cobra: contarlo mandaría al operador a perseguir dinero que nadie
  // debe, y a desconfiar del contador la próxima vez.
  it('lo cancelado no cuenta como por cobrar', () => {
    const r = resumenPorCobrar([orden({ id: 5, paid: false, enPreparacion: true, renglones: 1, total: '200', outstanding: '200', status: 'cancelada' })]);
    expect(r).toEqual({ cuantos: 0, monto: 0 });
  });

  it('sin nada pendiente el resumen queda en cero', () => {
    expect(resumenPorCobrar([orden({ paid: true })])).toEqual({ cuantos: 0, monto: 0 });
  });
});

// UN PEDIDO DE $0 NO ESTÁ SALDADO, PERO NO HAY NADA QUE COBRARLE.
//
// El badge filtraba por `!paid`, que el servidor descartó por escrito: `paid` exige un total
// positivo, así que un pedido de $0 llega con `paid: false` y `outstanding: "0"`. El badge decía
// "1 por cobrar · $0" y ninguna tarjeta ofrecía Cobrar, porque el botón se pinta con
// `outstanding > 0`. Un renglón que no se puede atender y que no se va solo.
test('un pedido de $0 no cuenta como por cobrar', () => {
  const cero = orden({ id: 99, status: 'entregada', paid: false, outstanding: '0', total: '0' });
  const debe = orden({ id: 100, status: 'entregada', paid: false, outstanding: '150', total: '150' });

  const pendientes = porCobrar([cero, debe]);

  expect(pendientes.map((o) => o.id)).toEqual([100]);
  expect(resumenPorCobrar([cero, debe])).toEqual({ cuantos: 1, monto: 150 });
});
