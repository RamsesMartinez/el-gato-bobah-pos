import type { ExpiryLot } from '../../../api/backoffice';
import { formatBaseQty } from '../../../shared/units';

// Los grupos de Caducidades (spec 098, maqueta A): rangos que no se enciman.
export const GROUPS = ['Ya caducó', 'Esta semana', '8 a 30 días', '31 a 60 días', 'Más adelante'] as const;
export type Group = (typeof GROUPS)[number];

export function groupOf(l: Pick<ExpiryLot, 'daysLeft'>): Group {
  if (l.daysLeft < 0) return 'Ya caducó';
  if (l.daysLeft <= 7) return 'Esta semana';
  if (l.daysLeft <= 30) return '8 a 30 días';
  if (l.daysLeft <= 60) return '31 a 60 días';
  return 'Más adelante';
}

// projection dice en una frase qué pasa con el lote; warn marca las que piden decidir algo.
export function projection(l: ExpiryLot): { text: string; warn: boolean } {
  if (l.expired) return { text: 'Caducó: mándalo a merma', warn: true };
  if (l.noRecentUse) return { text: 'No se ha vendido en 30 días', warn: true };
  if (l.leftoverAtExpiry != null) {
    return { text: `No se termina antes de caducar: quedan ~${formatBaseQty(l.leftoverAtExpiry, l.unitCode)}`, warn: true };
  }
  if (l.finishesInDays != null) return { text: `Se termina en ~${l.finishesInDays} ${l.finishesInDays === 1 ? 'día' : 'días'}`, warn: false };
  return { text: '', warn: false };
}

export function whenText(daysLeft: number): string {
  if (daysLeft < -1) return `caducó hace ${-daysLeft} días`;
  if (daysLeft === -1) return 'caducó ayer';
  if (daysLeft === 0) return 'caduca hoy';
  if (daysLeft === 1) return 'mañana';
  return `en ${daysLeft} días`;
}
