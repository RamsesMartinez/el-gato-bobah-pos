import { describe, expect, test } from 'vitest';
import { diffWords, entryTotal, isBigDiff, looseUnitOf, looseUnitsOf, packsTotal } from './countEntry';

describe('entryTotal', () => {
  test('empaques más suelto, en la unidad base', () => {
    // 3 botellas de 750 ml + 0.5 L abiertos = 2,750 ml.
    expect(entryTotal('3', '750', '0.5', 1000)).toBe(2750);
    expect(entryTotal('', null, '12', 1)).toBe(12);
  });
  test('las dos vacías es «no contado», no cero', () => {
    expect(entryTotal('', '750', '', 1000)).toBeNull();
  });
  test('un cero tecleado es «no hay»', () => {
    expect(entryTotal('0', '750', '', 1000)).toBe(0);
  });
  test('lo mal tecleado no se adivina', () => {
    expect(entryTotal('3,5', '750', '', 1000)).toBeNull();
  });
});

describe('looseUnitOf', () => {
  test('lo suelto se cuenta en la unidad grande; las piezas sin decimales', () => {
    expect(looseUnitOf('g')).toMatchObject({ label: 'kg', factor: 1000, decimals: true });
    expect(looseUnitOf('pieza')).toMatchObject({ label: 'piezas', decimals: false });
  });
});

describe('diffWords e isBigDiff', () => {
  test('sobraban o faltaban, nunca un negativo a secas', () => {
    expect(diffWords(4200, 'g')).toBe('sobraban 4.2 kg');
    expect(diffWords(-300, 'g')).toBe('faltaban 300 g');
  });
  test('una diferencia grande se marca; una chica no', () => {
    expect(isBigDiff(-300, 500, 800)).toBe(true);
    expect(isBigDiff(-20, 980, 1000)).toBe(false);
    expect(isBigDiff(4200, 3000, -1200)).toBe(true); // el sistema estaba en negativo
  });
});

// Spec 042, FR-012: el conteo ofrece todos los empaques, un selector de unidad que abre en la base
// y, en piezas con equivalencia de peso, g y kg para pesar lo suelto.
describe('looseUnitsOf', () => {
  it('abre en la unidad base y ofrece la grande', () => {
    expect(looseUnitsOf('g', false).map((u) => u.label)).toEqual(['g', 'kg']);
    expect(looseUnitsOf('ml', false).map((u) => u.label)).toEqual(['ml', 'L']);
    expect(looseUnitsOf('cm', false).map((u) => u.label)).toEqual(['cm', 'm']);
  });
  it('en piezas no hay selector, salvo que se pueda pesar', () => {
    expect(looseUnitsOf('pieza', false).map((u) => u.label)).toEqual(['piezas']);
    const w = looseUnitsOf('pieza', true);
    expect(w.map((u) => u.label)).toEqual(['piezas', 'g', 'kg']);
    expect(w[1].weightCode).toBe('g');
  });
});

describe('packsTotal', () => {
  it('suma cada empaque por lo que trae; vacío es «no contado»', () => {
    expect(packsTotal(['2', '', '1'], ['5000', '15000', '1000'])).toBe(11000);
    expect(packsTotal(['', ''], ['5000', '15000'])).toBeNull();
    expect(packsTotal(['0'], ['5000'])).toBe(0);
  });
});
