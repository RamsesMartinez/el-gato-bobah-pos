// Clases de unidad de un artículo y cómo se capturan sus cantidades (spec 042).

export type Kind = 'masa' | 'volumen' | 'longitud' | 'pieza';

export const KINDS: { kind: Kind; label: string }[] = [
  { kind: 'masa', label: 'Peso' },
  { kind: 'volumen', label: 'Líquido' },
  { kind: 'longitud', label: 'Longitud' },
  { kind: 'pieza', label: 'Piezas' },
];

export const SMALL: Record<Kind, string> = { masa: 'g', volumen: 'ml', longitud: 'cm', pieza: 'piezas' };
export const BIG: Partial<Record<Kind, { label: string; factor: number }>> = {
  masa: { label: 'kg', factor: 1000 },
  volumen: { label: 'L', factor: 1000 },
  longitud: { label: 'm', factor: 100 },
};

// bigUnitOf es la unidad grande de una clase y cuántas de la base trae; las piezas no tienen.
export const bigUnitOf = (k: Kind) => BIG[k] ?? null;

// baseQty pasa lo capturado a la unidad base. Vacío es «no se capturó» (undefined), nunca 0: borrar
// el campo para reescribirlo no puede guardar un cero.
export function baseQty(qty: string, kind: Kind, big: boolean): string | undefined {
  if (qty.trim() === '') return undefined;
  const n = Number(qty);
  if (!Number.isFinite(n)) return undefined;
  const f = big ? BIG[kind]?.factor ?? 1 : 1;
  return String(Math.round(n * f * 10000) / 10000);
}

// Cómo se dice la clase de unidad a quien opera.
export const KIND_WORD: Record<string, string> = { masa: 'peso', volumen: 'líquido', longitud: 'longitud', pieza: 'piezas' };

// Los empaques que se ofrecen siempre al agregar uno, por clase de unidad (spec 043, FR-014). Genéricos:
// nada propio de un negocio. Editarlos por empresa es #121.
export const BASE_PACKS: Partial<Record<string, string[]>> = {
  masa: ['Bolsa', 'Caja', 'Costal', 'Paquete'],
  volumen: ['Botella', 'Galón', 'Garrafa', 'Caja'],
  pieza: ['Caja', 'Paquete', 'Bolsa'],
};
