import { ApiError } from '../../api/client';
import type { CashRegister } from '../../api/backoffice';

// activeRegisters son las cajas de las que puede salir el efectivo de un gasto: todas las activas,
// tengan turno o no (spec 047). Si el pago no entra a un corte lo decide el servidor, no la lista.
export function activeRegisters(items: CashRegister[] | undefined): CashRegister[] {
  return (items ?? []).filter((r) => r.isActive !== false);
}

// registerLabel nombra la caja con lo que le pasará al efectivo: sin sufijo entra al corte abierto.
export function registerLabel(r: CashRegister): string {
  if (r.countsCash === false) return `${r.name} · sin arqueo`;
  if (r.openSessionId === null) return `${r.name} · sin turno`;
  return r.name;
}

export const isOutsideCount = (e: unknown) => e instanceof ApiError && e.code === 'EXPENSE_OUTSIDE_COUNT';

// outsideTitle dice por qué el pago queda fuera: la caja sin turno, o (con turno) un pago de otro día.
export function outsideTitle(registers: CashRegister[], ids: (number | undefined)[]): string {
  const closed = registers.find((r) => ids.includes(r.id) && r.countsCash !== false && r.openSessionId === null);
  return closed ? `${closed.name} no tiene turno abierto` : 'El pago es de otro día';
}

export const OUTSIDE_COUNT_TEXT = 'Este gasto no entrará a ningún corte.';
export const OUTSIDE_COUNT_CONFIRM = 'Registrar sin incluir en el arqueo';
