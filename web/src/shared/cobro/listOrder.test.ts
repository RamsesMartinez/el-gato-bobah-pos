import { describe, expect, it } from 'vitest';
import type { OrderLine, PaymentView } from '../../types/pos';
import { listOrder, PAID_GROUP_THRESHOLD } from './listOrder';

const line = (id: number, quantity: string, over: Partial<OrderLine> = {}): OrderLine => ({
  id, productName: `Producto ${id}`, quantity, unitPrice: '10', lineTotal: '10', delivered: '0', cancelled: false, ...over,
});
const payment = (number: number, lines: Array<[number, string]>, over: Partial<PaymentView> = {}): PaymentView => ({
  id: number * 100, number, voided: false, methodId: 1, methodName: 'Efectivo', amount: '10', tip: '0', reference: '',
  paidAt: '', receivedBy: '', split: null, lines: lines.map(([lineId, qty]) => ({ lineId, qty, amount: '10' })), ...over,
});

describe('listOrder', () => {
  // Lo que falta pagar va arriba: es lo que quien cobra tiene que tocar. Lo pagado sigue a la vista,
  // al final, para que nadie crea que desapareció.
  it('pone lo pendiente arriba y lo pagado al final, en el orden del pedido', () => {
    const r = listOrder([line(1, '1'), line(2, '1'), line(3, '1')], [payment(1, [[2, '1']])]);
    expect(r.pending.map((x) => x.line.id)).toEqual([1, 3]);
    expect(r.paid.map((x) => x.line.id)).toEqual([2]);
    expect(r.paid[0].paidBy).toEqual([1]);
  });

  it('un renglón con piezas por pagar queda arriba con las que faltan', () => {
    const r = listOrder([line(1, '2')], [payment(1, [[1, '1']])]);
    expect(r.pending).toHaveLength(1);
    expect(r.pending[0].free).toBe(1);
    expect(r.pending[0].paid).toBe(1);
    expect(r.paid).toHaveLength(0);
  });

  it('lo cancelado no se lista y un pago devuelto no cubre nada', () => {
    const r = listOrder([line(1, '1', { cancelled: true }), line(2, '1')], [payment(1, [[2, '1']], { voided: true })]);
    expect(r.pending.map((x) => x.line.id)).toEqual([2]);
    expect(r.paid).toHaveLength(0);
  });

  // Con muchos pagados, la lista se los come: se agrupan en «N pagados ▸».
  it(`agrupa lo pagado cuando pasa de ${PAID_GROUP_THRESHOLD}`, () => {
    const lines = [1, 2, 3, 4, 5, 6].map((i) => line(i, '1'));
    expect(listOrder(lines, [payment(1, [[1, '1'], [2, '1'], [3, '1'], [4, '1']])]).groupPaid).toBe(false);
    expect(listOrder(lines, [payment(1, [[1, '1'], [2, '1'], [3, '1'], [4, '1'], [5, '1']])]).groupPaid).toBe(true);
  });
});
