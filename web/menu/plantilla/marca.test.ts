import { describe, expect, it } from 'vitest';
import { PALETAS, contraste } from './marca';

// EL COLOR DEL NEGOCIO SE LEE CON SOL EN EL CELULAR (revisión de pantallas, spec 097). El texto
// blanco sobre el encabezado y el precio sobre fondo blanco usan el tono 700: con el 600, verde,
// naranja, cielo y menta quedaban bajo 4.5:1.
describe('las 8 paletas del menú', () => {
  for (const [id, t] of Object.entries(PALETAS)) {
    it(`${id}: blanco sobre el encabezado y precio sobre blanco ≥ 4.5:1`, () => {
      expect(contraste('#ffffff', t.c700), `${id}: blanco sobre ${t.c700}`).toBeGreaterThanOrEqual(4.5);
    });
  }
  it('son las mismas que la pantalla Interfaz', () => {
    expect(Object.keys(PALETAS)).toEqual(['brand', 'blue', 'teal', 'green', 'cyan', 'purple', 'pink', 'orange']);
  });
});
