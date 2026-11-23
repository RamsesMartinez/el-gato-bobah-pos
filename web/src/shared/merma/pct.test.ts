import { describe, expect, test } from 'vitest';
import { pctTecleado, pctTexto } from './pct';

// La merma esperada (spec 096): un campo vacío es «sin captura» y viaja como null, nunca como 0
// (Number('') es 0 y un 0 capturado significa «aquí no se pierde nada»).
describe('pctTecleado', () => {
  test.each([
    ['', null],
    ['   ', null],
    ['0', 0],
    ['5', 5],
    ['2.5', 2.5],
    ['2,5', 2.5],
    ['99.99', 99.99],
  ])('«%s» → %s', (t, want) => expect(pctTecleado(t)).toBe(want));

  test.each(['100', '-1', 'cinco', '2.555', '5 %x'])('«%s» es inválido', (t) => {
    expect(pctTecleado(t)).toBe('invalido');
  });
});

describe('pctTexto', () => {
  test('quita los ceros que sobran y deja vacío lo nulo', () => {
    expect(pctTexto('5.00')).toBe('5');
    expect(pctTexto('2.50')).toBe('2.5');
    expect(pctTexto('0.00')).toBe('0');
    expect(pctTexto(null)).toBe('');
    expect(pctTexto(undefined)).toBe('');
  });
});
