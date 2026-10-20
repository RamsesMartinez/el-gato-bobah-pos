import type { Unit } from '../api/backoffice';
import { round2 } from '../domain/numeros';

// Unidades del almacén (spec 038). Lo guardado se cuenta en g, ml, cm o pieza; a quien opera se le
// muestra en kg, L o m en cuanto llega a una unidad grande, y nunca con decimales rotos.

const BIG: Record<string, { at: number; unit: string }> = {
  g: { at: 1000, unit: 'kg' },
  ml: { at: 1000, unit: 'L' },
  cm: { at: 100, unit: 'm' },
};

// Un número, no una fecha: no lleva zona (ver formateoUnico.test.ts).
const num = (v: number) => { const n = round2(v); return n.toLocaleString('es-MX', { maximumFractionDigits: 2 }); };

// formatBaseQty muestra una cantidad en unidad base («12000», «ml») como la diría alguien
// («12 L»). Los negativos se muestran tal cual: quien decide cómo decirlos es la pantalla.
export function formatBaseQty(qty: string | number, baseUnitCode: string): string {
  const n = typeof qty === 'number' ? qty : Number(qty);
  if (baseUnitCode === 'pieza') return `${num(n)} ${round2(Math.abs(n)) === 1 ? 'pieza' : 'piezas'}`;
  const big = BIG[baseUnitCode];
  if (big && Math.abs(n) >= big.at) return `${num(n / big.at)} ${big.unit}`;
  return `${num(n)} ${baseUnitCode}`;
}

// Las unidades de receta (cucharadas, onzas) no se ofrecen al comprar (regla 9).
const NOT_FOR_BUYING = new Set(['cda', 'cdta', 'floz']);

// unitLabel es como se dice la unidad en un botón: «L» y no «l».
export const unitLabel = (u: Unit) => (u.code === 'l' ? 'L' : u.code === 'pieza' ? 'piezas' : u.code);

// purchaseUnitsFor devuelve las unidades de compra de un tipo, de la más grande a la más chica.
export function purchaseUnitsFor(kind: string, units: Unit[]): Unit[] {
  return units
    .filter((u) => u.kind === kind && !NOT_FOR_BUYING.has(u.code))
    .sort((a, b) => Number(b.toBase) - Number(a.toBase));
}

// formatUnitCost muestra un costo por unidad base («0.006», «g») como se compra: «$6.00 / kg».
// Sin costo devuelve null: la pantalla decide cómo decirlo («Sin costo»).
export function formatUnitCost(cost: string | number, baseUnitCode: string): string | null {
  const n = typeof cost === 'number' ? cost : Number(cost);
  if (!Number.isFinite(n) || n <= 0) return null;
  const big = BIG[baseUnitCode];
  const per = big ? big.unit : baseUnitCode === 'pieza' ? 'pieza' : baseUnitCode;
  return `$${money(big ? n * big.at : n)} / ${per}`;
}

// Un número, no una fecha: no lleva zona (ver formateoUnico.test.ts).
const money = (n: number) => n.toLocaleString('es-MX', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

// plainQty es una cantidad en la unidad base tal cual, con separador de miles: «5,000».
export const plainQty = (v: string | number) => { const n = Number(v); return n.toLocaleString('es-MX', { maximumFractionDigits: 4 }); };

// packExample es el ejemplo de empaque que se da al explicar qué es uno, de la clase del artículo: un
// líquido no se compra en bolsas de 5 kg.
export function packExample(kind: string): string {
  switch (kind) {
    case 'masa': return 'bolsa de 5 kg';
    case 'volumen': return 'garrafa de 4 L';
    case 'longitud': return 'rollo de 50 m';
    default: return 'caja de 12 piezas';
  }
}
