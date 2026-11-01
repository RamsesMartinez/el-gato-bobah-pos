import type { ExpenseDetail, ExpenseItem } from '../../api/backoffice';
import { parseNumero, round2 } from '../../domain/numeros';

// Piezas compartidas de la edición de un gasto (spec 053).

const MESES = ['ene', 'feb', 'mar', 'abr', 'may', 'jun', 'jul', 'ago', 'sep', 'oct', 'nov', 'dic'];

// dia escribe un día AAAA-MM-DD como «20 oct».
export function dia(iso: string): string {
  const [, m, d] = iso.split('-').map(Number);
  return `${d} ${MESES[m - 1]}`;
}

const num = (s: string | null | undefined) => {
  const n = parseNumero(s ?? '');
  return n.estado === 'valido' ? n.valor : 0;
};

// missingOf es lo que falta por llegar de un renglón, en su unidad.
export const missingOf = (i: ExpenseItem) => round2(num(i.quantity) - num(i.qtyReceived) - num(i.notComing));

// pendingGoods son los renglones de almacén a los que todavía les falta algo.
export function pendingGoods(e: ExpenseDetail): ExpenseItem[] {
  return e.items.filter((i) => i.itemType !== null && !i.excluded && missingOf(i) > 0);
}
