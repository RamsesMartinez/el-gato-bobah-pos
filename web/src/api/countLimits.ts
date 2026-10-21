import { api } from './client';

// Límites del conteo por empresa (spec 044): por clase de unidad, lo más que se registra de un
// artículo en un conteo y desde qué cifra se pide confirmar. Viajan en unidad base (g, ml, cm, pieza);
// en pantalla se dicen en kg, L, m y piezas.

export type UnitClass = 'mass' | 'volume' | 'length' | 'piece';
export interface ClassLimits { max: number; confirm: number }
export type CountLimits = Record<UnitClass, ClassLimits>;

export const UNIT_CLASSES: UnitClass[] = ['mass', 'volume', 'length', 'piece'];

// Los mismos que siembra el servidor: si la lectura falla, el aviso usa éstos y el servidor decide.
export const DEFAULT_COUNT_LIMITS: CountLimits = {
  mass: { max: 1_000_000, confirm: 50_000 },
  volume: { max: 1_000_000, confirm: 50_000 },
  length: { max: 1_000_000, confirm: 5_000 },
  piece: { max: 1_000_000, confirm: 5_000 },
};

// La unidad grande de cada clase y cuántas unidades base lleva.
export const BIG_UNIT: Record<UnitClass, { label: string; factor: number; name: string }> = {
  mass: { label: 'kg', factor: 1000, name: 'Peso' },
  volume: { label: 'L', factor: 1000, name: 'Líquido' },
  length: { label: 'm', factor: 100, name: 'Longitud' },
  piece: { label: 'piezas', factor: 1, name: 'Piezas' },
};

const CLASS_OF: Record<string, UnitClass> = { g: 'mass', ml: 'volume', cm: 'length', pieza: 'piece' };

export const unitClassOf = (baseUnitCode: string): UnitClass | null => CLASS_OF[baseUnitCode] ?? null;

export const formatThousands = (n: number) => n.toLocaleString('es-MX', { maximumFractionDigits: 4 });

// overMaxMessage dice «Máximo N <unidad> por conteo» si lo contado (en unidad base) pasa el máximo
// de su clase; null si cabe o si la unidad no tiene clase (el servidor decide).
export function overMaxMessage(limits: CountLimits, baseUnitCode: string, qtyInBase: number): string | null {
  const cls = unitClassOf(baseUnitCode);
  if (!cls || !(qtyInBase > limits[cls].max)) return null;
  const big = BIG_UNIT[cls];
  return `Máximo ${formatThousands(limits[cls].max / big.factor)} ${big.label} por conteo`;
}

export const countLimitsApi = {
  get: () => api.get<CountLimits>('/settings/count-limits'),
  save: (l: CountLimits) => api.put<CountLimits>('/settings/count-limits', l),
};
