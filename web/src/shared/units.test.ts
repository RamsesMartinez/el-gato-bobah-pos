import { describe, expect, test } from 'vitest';
import { formatBaseQty, purchaseUnitsFor } from './units';
import type { Unit } from '../api/backoffice';

const units: Unit[] = [
  { id: 1, code: 'g', name: 'Gramo', kind: 'masa', toBase: '1' },
  { id: 2, code: 'kg', name: 'Kilogramo', kind: 'masa', toBase: '1000' },
  { id: 3, code: 'ml', name: 'Mililitro', kind: 'volumen', toBase: '1' },
  { id: 4, code: 'cdta', name: 'Cucharadita', kind: 'volumen', toBase: '5' },
  { id: 5, code: 'cda', name: 'Cucharada', kind: 'volumen', toBase: '15' },
  { id: 6, code: 'floz', name: 'Onza líquida', kind: 'volumen', toBase: '29.5735' },
  { id: 7, code: 'l', name: 'Litro', kind: 'volumen', toBase: '1000' },
  { id: 8, code: 'pieza', name: 'Pieza', kind: 'pieza', toBase: '1' },
  { id: 9, code: 'cm', name: 'Centímetro', kind: 'longitud', toBase: '1' },
  { id: 10, code: 'm', name: 'Metro', kind: 'longitud', toBase: '100' },
];

// Lo guardado se cuenta en g, ml, cm o pieza; se muestra en kg, L o m desde 1,000 (o 100 cm), con
// redondeo y sin decimales rotos («11.399999999999999 L» salió en una maqueta).
describe('formatBaseQty (regla 10 de unidades)', () => {
  test.each([
    ['12000', 'ml', '12 L'],
    ['3785.4', 'ml', '3.79 L'],
    ['750', 'ml', '750 ml'],
    ['25000', 'g', '25 kg'],
    ['280', 'g', '280 g'],
    ['10000', 'cm', '100 m'],
    ['35', 'cm', '35 cm'],
    ['50', 'pieza', '50 piezas'],
    ['1', 'pieza', '1 pieza'],
    ['-200', 'ml', '-200 ml'],
    ['11399.999999999999', 'ml', '11.4 L'],
  ])('%s %s → %s', (qty, unit, want) => {
    expect(formatBaseQty(qty, unit)).toBe(want);
  });
});

// Al comprar no se ofrecen cucharadas ni onzas (regla 9), y solo unidades del mismo tipo.
describe('purchaseUnitsFor', () => {
  test('volumen: L y ml, sin cucharadas ni onzas', () => {
    expect(purchaseUnitsFor('volumen', units).map((u) => u.code)).toEqual(['l', 'ml']);
  });
  test('masa: kg y g', () => {
    expect(purchaseUnitsFor('masa', units).map((u) => u.code)).toEqual(['kg', 'g']);
  });
  test('longitud: m y cm', () => {
    expect(purchaseUnitsFor('longitud', units).map((u) => u.code)).toEqual(['m', 'cm']);
  });
  test('pieza: piezas', () => {
    expect(purchaseUnitsFor('pieza', units).map((u) => u.code)).toEqual(['pieza']);
  });
});
