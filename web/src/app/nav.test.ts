import { describe, it, expect } from 'vitest';
import { NAV } from './nav';

// LA BARRA LATERAL: en la tableta de 600 px solo se ven los primeros 8 sin desplazar.
describe('la barra lateral', () => {
  const labels = NAV.map((n) => n.label);

  // «Catálogo» no decía que ahí estaban los productos y las recetas, y quedaba abajo del corte de la
  // tableta: nadie la encontraba (decisión del dueño, 2026-10-07).
  it('el menú se llama «Menú» y se ve sin desplazar', () => {
    expect(labels).not.toContain('Catálogo');
    expect(labels.indexOf('Menú')).toBeGreaterThanOrEqual(0);
    expect(labels.indexOf('Menú')).toBeLessThan(8);
  });

  it('Almacén queda junto al menú: las recetas descuentan de ahí', () => {
    expect(labels.indexOf('Almacén')).toBe(labels.indexOf('Menú') + 1);
  });
});
