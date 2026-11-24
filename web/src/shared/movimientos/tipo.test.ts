import { describe, expect, test } from 'vitest';
import { tipoDeMovimiento } from './tipo';

// Movimientos mostraba el tipo tal como lo guarda la base («cancelacion», «merma_esperada»).
describe('tipoDeMovimiento', () => {
  test.each([
    ['venta', 'Venta'], ['compra', 'Compra'], ['ajuste', 'Ajuste'], ['merma', 'Merma'],
    ['produccion', 'Producción'], ['cancelacion', 'Cancelación'], ['traspaso', 'Traspaso'],
    ['merma_esperada', 'Merma esperada'],
  ])('%s → %s', (t, w) => expect(tipoDeMovimiento(t)).toBe(w));

  test('un tipo que todavía no conoce se muestra legible, sin tronar', () => {
    expect(tipoDeMovimiento('algo_nuevo')).toBe('Algo nuevo');
  });
});
