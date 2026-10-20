import { describe, it, expect } from 'vitest';
import { existingByName, nextTempId } from './createIngredient';

// Un insumo que ya existe con otras mayúsculas o acentos se elige en vez de crear otro (spec 037), y
// uno nuevo lleva un id provisional negativo hasta que su captura se guarda (spec 042, FR-003).
describe('existingByName', () => {
  const existing = [{ id: 4, name: 'Lata Coca' }, { id: 5, name: 'Leche' }, { id: -2, name: 'Jarabe' }];

  it('encuentra el que ya se llama así sin mayúsculas, acentos ni espacios de más', () => {
    expect(existingByName('láta  coca', existing)).toEqual({ id: 4, name: 'Lata Coca' });
  });
  it('sin uno parecido no inventa', () => {
    expect(existingByName('Otro', existing)).toBeUndefined();
  });
  it('uno provisional (todavía sin guardar) no cuenta como existente', () => {
    expect(existingByName('Jarabe', existing)).toBeUndefined();
  });
});

describe('nextTempId', () => {
  it('da ids negativos y distintos', () => {
    const a = nextTempId();
    const b = nextTempId();
    expect(a).toBeLessThan(0);
    expect(b).toBeLessThan(0);
    expect(a).not.toBe(b);
  });
});
