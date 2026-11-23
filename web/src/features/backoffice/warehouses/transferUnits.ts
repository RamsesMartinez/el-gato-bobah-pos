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

export function typedUnit(baseCode: string): TypedUnit {
  return UNITS[baseCode] ?? { label: baseCode, factor: 1, decimals: true };
}

// toBase convierte lo tecleado a la unidad base, con los 4 decimales de la columna. Lo que no es un
// número positivo es cero: no se manda.
export function toBase(typed: string, baseCode: string): number {
  const n = Number(typed);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.round(n * typedUnit(baseCode).factor * 10000) / 10000;
}
