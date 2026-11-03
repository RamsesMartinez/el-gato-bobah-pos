import { describe, expect, test } from 'vitest';
import { cajasConTurno } from './cajaDelGasto';

describe('cajasConTurno', () => {
  test('una caja sin arqueo no sale entre las que abren turno: se vería «Cerrada» para siempre', () => {
    const cajas = [{ id: 1, countsCash: true }, { id: 6, countsCash: false }, { id: 2 }];
    expect(cajasConTurno(cajas).map((c) => c.id)).toEqual([1, 2]);
  });
});
