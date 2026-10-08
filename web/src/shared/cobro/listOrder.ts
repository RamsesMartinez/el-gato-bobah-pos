import type { OrderLine, PaymentView } from '../../types/pos';

// PAID_GROUP_THRESHOLD es a partir de cuántos productos pagados la lista los agrupa en una fila
// «N pagados ▸». En 600 px de alto, once renglones pagados empujan fuera de la pantalla los que
// todavía hay que tocar.
export const PAID_GROUP_THRESHOLD = 4;

// ListRowState es un renglón vivo del pedido con lo que la hoja necesita para pintarlo.
export interface ListRowState {
  line: OrderLine;
  // Piezas que ya cubren pagos vivos, y las que quedan por pagar.
  paid: number;
  free: number;
  // Los números de los pagos que lo cubren, para «Pago 2 · Tarjeta».
  paidBy: number[];
}

export interface ListOrder {
  pending: ListRowState[];
  paid: ListRowState[];
  groupPaid: boolean;
}

// listOrder ordena los renglones vivos para la hoja de cobro: lo que falta pagar arriba, en el orden
// del pedido, y lo pagado al final. Un pago devuelto ya no cubre nada.
export function listOrder(lines: OrderLine[], payments: PaymentView[]): ListOrder {
  const paidQty = new Map<number, number>();
  const paidBy = new Map<number, number[]>();
  for (const p of payments) {
    if (p.voided) continue;
    for (const l of p.lines) {
      paidQty.set(l.lineId, (paidQty.get(l.lineId) ?? 0) + Number(l.qty));
      paidBy.set(l.lineId, [...(paidBy.get(l.lineId) ?? []), p.number]);
    }
  }
  const pending: ListRowState[] = [];
  const paid: ListRowState[] = [];
  for (const line of lines) {
    if (line.cancelled) continue;
    const covered = paidQty.get(line.id) ?? 0;
    const row = { line, paid: covered, free: Math.max(0, Number(line.quantity) - covered), paidBy: paidBy.get(line.id) ?? [] };
    (row.free > 0 ? pending : paid).push(row);
  }
  return { pending, paid, groupPaid: paid.length > PAID_GROUP_THRESHOLD };
}
