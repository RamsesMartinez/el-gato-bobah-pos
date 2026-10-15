import { describe, expect, test } from 'vitest';
import { diffWords, entryTotal, isBigDiff, looseUnitOf } from './countEntry';

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
