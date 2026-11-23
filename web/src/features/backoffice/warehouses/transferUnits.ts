// Unidades con que se teclea un traspaso (spec 094): lo guardado va en g, ml, cm o pieza, pero quien
// lleva leche de la bodega piensa en litros. Se teclea en la unidad grande y se manda en la base.

export interface TypedUnit {
  label: string;
  // Cuánto de la unidad base es una unidad tecleada.
  factor: number;
  decimals: boolean;
}

const UNITS: Record<string, TypedUnit> = {
  g: { label: 'kg', factor: 1000, decimals: true },
  ml: { label: 'L', factor: 1000, decimals: true },
  cm: { label: 'm', factor: 100, decimals: true },
  pieza: { label: 'pz', factor: 1, decimals: false },
};

// typedUnit es la unidad en que se teclea. Con `shown` (lo que hay en el origen), sigue a como se ve
// esa existencia (formatBaseQty): con menos de una unidad grande se ve «105 g», y teclear en kg
// llevaba 2 kg de algo que solo tenía 105 g.
export function typedUnit(baseCode: string, shown?: string | number): TypedUnit {
  const big = UNITS[baseCode] ?? { label: baseCode, factor: 1, decimals: true };
  if (shown !== undefined && big.factor > 1 && Math.abs(Number(shown)) < big.factor) return { label: baseCode, factor: 1, decimals: true };
  return big;
}

// toBase convierte lo tecleado a la unidad base, con los 4 decimales de la columna. Lo que no es un
// número positivo es cero: no se manda.
export function toBase(typed: string, baseCode: string, shown?: string | number): number {
  const n = Number(typed);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.round(n * typedUnit(baseCode, shown).factor * 10000) / 10000;
}

// KEY_REUSED_MESSAGE se dice cuando la clave del traspaso ya se usó con otro cuerpo: casi siempre el
// primer envío sí entró y su respuesta se perdió, así que repetirlo a ciegas movería la mercancía dos
// veces.
export const KEY_REUSED_MESSAGE = 'Ese traspaso ya se había hecho. Revisa las existencias antes de repetirlo.';

// richestSource es el almacén que más tiene del artículo: el que «Traer de…» propone.
export function richestSource<T extends { onHand: string }>(sources: T[]): T {
  return sources.reduce((a, b) => (Number(b.onHand) > Number(a.onHand) ? b : a));
}
