import { describe, expect, test } from 'vitest';
import { toBase, typedUnit } from './transferUnits';

describe('unidades del traspaso', () => {
  test('se teclea en la unidad grande y se manda en la base', () => {
    expect(toBase('6', 'ml')).toBe(6000);
    expect(toBase('2.5', 'g')).toBe(2500);
    expect(toBase('3', 'pieza')).toBe(3);
    expect(typedUnit('ml').label).toBe('L');
  });

  // El vacío, el cero y lo que no es número no se mandan (un renglón en cero no es un traspaso).
  test('lo que no es una cantidad positiva es cero', () => {
    expect(toBase('', 'ml')).toBe(0);
    expect(toBase('0', 'ml')).toBe(0);
    expect(toBase('-1', 'ml')).toBe(0);
    expect(toBase('.', 'ml')).toBe(0);
  });

  // Con menos de una unidad grande en el origen, se teclea en la base: la existencia se ve «105 g».
  test('la unidad sigue a como se ve la existencia', () => {
    expect(typedUnit('g', '105').label).toBe('g');
    expect(toBase('2', 'g', '105')).toBe(2);
    expect(typedUnit('g', '25000').label).toBe('kg');
    expect(toBase('2', 'g', '25000')).toBe(2000);
    expect(typedUnit('cm', '50').label).toBe('cm');
    expect(typedUnit('pieza', '3').label).toBe('pz');
  });
});
