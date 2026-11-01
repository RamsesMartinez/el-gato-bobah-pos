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
  if (r.openSessionId === null) return `${r.name} · cerrada`;
  return r.name;
}

export const isOutsideCount = (e: unknown) => e instanceof ApiError && e.code === 'EXPENSE_OUTSIDE_COUNT';
export const isBeforeLastClose = (e: unknown) => e instanceof ApiError && e.code === 'EXPENSE_BEFORE_LAST_CLOSE';

// countsInDrawer dice si el pago lleva la casilla «Incluir en el arqueo»: efectivo de una caja con
// arqueo. Tarjeta, transferencia y cajas sin arqueo nunca entran a un corte (spec 048).
export function countsInDrawer(isCash: boolean, registers: CashRegister[], registerId: number | undefined): boolean {
  if (!isCash || !registerId) return false;
  const r = registers.find((x) => x.id === registerId);
  return !!r && r.countsCash !== false;
}

// OutsideAsk es la hoja de aviso: `canInclude` es falso cuando la hora del pago ya quedó antes del
// último cierre y ningún corte la puede cubrir.
export interface OutsideAsk { title: string; canInclude: boolean }

export function outsideAskOf(e: unknown, registers: CashRegister[], ids: (number | undefined)[]): OutsideAsk | null {
  if (isOutsideCount(e)) return { title: outsideTitle(registers, ids), canInclude: true };
  if (isBeforeLastClose(e)) return { title: 'El pago es de antes del último corte', canInclude: false };
  return null;
}

// CountChoice es la respuesta a la hoja: «pending» espera al corte que cubra su hora; «exclude» lo
// deja fuera para siempre.
export type CountChoice = 'pending' | 'exclude';

// withCountChoice aplica la respuesta a los pagos que llevan la casilla marcada: son los únicos que
// pudieron provocar el aviso.
export function withCountChoice<T extends { includeInCount?: boolean; outsideCount?: boolean }>(p: T, choice?: CountChoice): T {
  if (!choice || p.includeInCount !== true) return p;
  return choice === 'pending' ? { ...p, outsideCount: true } : { ...p, includeInCount: false };
}

// outsideTitle dice por qué el pago queda fuera: la caja sin turno, o (con turno) un pago de otro día.
export function outsideTitle(registers: CashRegister[], ids: (number | undefined)[]): string {
  const closed = registers.find((r) => ids.includes(r.id) && r.countsCash !== false && r.openSessionId === null);
  return closed ? `${closed.name} no tiene turno abierto` : 'El pago es de otro día';
}

export const OUTSIDE_COUNT_TEXT =
  'Este gasto no saldrá en ningún corte, salvo que el siguiente corte se abra con una hora de apertura que lo incluya.';
export const BEFORE_LAST_CLOSE_TEXT = 'La caja ya se cerró después de este pago: no puede entrar a ningún corte.';
export const INCLUDE_IN_COUNT_LABEL = 'Incluir en el arqueo';
export const OUTSIDE_COUNT_CONFIRM = 'Registrar sin incluir en el arqueo';

// cajasConTurno deja las cajas que se cuentan: una caja sin arqueo nunca abre turno.
export function cajasConTurno<T extends { countsCash?: boolean }>(cajas: T[]): T[] {
  return cajas.filter((c) => c.countsCash !== false);
}

export const DRAWER_OUT_LABEL = 'Salió de la caja';

// drawerExtraOf traduce «Salió de la caja» a la diferencia que viaja al servidor (spec 050). `out`
// null = no lo tocaron: salió lo mismo que el importe. Vacío no es cero: es un campo sin llenar, y
// no se guarda. Va entre el importe y el importe más lo excluido disponible; el servidor lo repite.
export function drawerExtraOf(amount: string, out: string | null, available: number): { extra?: string; bad: boolean } {
  if (out === null) return { bad: false };
  const a = Number(amount) || 0;
  const o = out.trim() === '' ? NaN : Number(out);
  if (!Number.isFinite(o) || o < a - 0.001 || o > a + available + 0.001) return { bad: true };
  const extra = o - a;
  return extra >= 0.005 ? { extra: extra.toFixed(2), bad: false } : { bad: false };
}
