import { readFileSync, statSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { FOOD_FAMILIES, FAMILY_SPRITE, familyLabel, familySymbol } from './foodFamilies';

const sprite = resolve(__dirname, '../../public', FAMILY_SPRITE.replace(/^\//, ''));

describe('familias de comida', () => {
  it('son las mismas once del servidor (domain.FoodFamilies) y en el mismo orden', () => {
    // Si una cambia allá y no aquí, la categoría guarda una familia que la pantalla no sabe dibujar.
    expect(FOOD_FAMILIES.map((f) => f.id)).toEqual([
      'cold_drink', 'coffee', 'tea', 'frappe', 'soda', 'soup', 'snack', 'wings', 'dessert', 'breakfast', 'dish',
    ]);
  });

  it('el sprite tiene un dibujo por familia y pesa menos de 40 KB (SC-003)', () => {
    const svg = readFileSync(sprite, 'utf8');
    for (const f of FOOD_FAMILIES) {
      expect(svg, `falta el dibujo de ${f.id}`).toContain(`<symbol id="${f.id}"`);
    }
    expect(statSync(sprite).size).toBeLessThan(40 * 1024);
  });

  it('una familia vacía o desconocida se dibuja como platillo, nunca como hueco', () => {
    expect(familySymbol('')).toBe('dish');
    expect(familySymbol(null)).toBe('dish');
    expect(familySymbol('taco')).toBe('dish');
    expect(familySymbol('coffee')).toBe('coffee');
    expect(familyLabel('')).toBe('Platillo (genérica)');
    expect(familyLabel('tea')).toBe('Té o bubble tea');
  });
});
