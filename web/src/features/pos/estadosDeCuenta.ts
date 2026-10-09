import type { AccountItem } from '../../types/pos';
import { money } from '../../utils/format';

export { ESTADO, nombreDeCuenta } from '../../domain/cuentas';

// Lo que se lee debajo del estado: lo que importa para cobrar.
export function loQueFalta(c: AccountItem): string {
  const falta = Number(c.outstanding);
  switch (c.state) {
    case 'capturing':
    case 'in_kitchen':
      return money(Number(c.total));
    case 'paid_in_kitchen':
      return 'pagada';
    case 'partly_paid':
      return `falta ${money(falta)}`;
    case 'delivered_owes':
      return `debe ${money(falta)}`;
  }
}

// La fila: fichas de ANCHO FIJO y solo las que caben completas. 120 px por ficha, 6 de separación
// y los dos botones de 44 («+N» y «+») siempre visibles.
export const FICHA = 120;
export const BOTON = 44;
export const GAP = 6;

export function fichasQueCaben(ancho: number): number {
  return Math.max(0, Math.floor((ancho - 2 * BOTON - GAP) / (FICHA + GAP)));
}

// La seleccionada primero (es la que se está atendiendo), luego las que deben dinero, luego por
// antigüedad. Una cuenta que se captura no debe todavía.
export function ordenDeLaFila(cuentas: AccountItem[], seleccionada: string | null): AccountItem[] {
  const debe = (c: AccountItem) => c.kind === 'order' && Number(c.outstanding) > 0;
  return [...cuentas].sort((a, b) => {
    if (a.key === seleccionada) return -1;
    if (b.key === seleccionada) return 1;
    if (debe(a) !== debe(b)) return debe(a) ? -1 : 1;
    return a.openedAt.localeCompare(b.openedAt);
  });
}

export function antiguedad(desde: string, ahora: number): string {
  const min = Math.floor((ahora - Date.parse(desde)) / 60_000);
  if (min < 1) return 'hace un momento';
  if (min < 60) return `hace ${min} min`;
  return `hace ${Math.floor(min / 60)} h`;
}

// Una cuenta vacía se descarta sin preguntar: no tiene nada que perder (D-7).
export function hayQuePreguntar(productos: number): boolean {
  return productos > 0;
}
