import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// EL DIÁLOGO HACE SCROLL POR DENTRO Y «GUARDAR» SE QUEDA A LA VISTA.
//
// Sin `scrollBehavior="inside"`, Chakra desplaza el diálogo entero como una página: el de producto
// ya pasaba de 600 px y «Guardar» quedaba abajo, fuera de la tableta. Lo encontró la revisión de
// tableta de la spec 028, antes de agregarle «Qué lleva».
//
// Es una prueba estática y no de estilos porque en jsdom las variantes de la receta del diálogo no
// llegan a `getComputedStyle`: el cuerpo sale igual con o sin la propiedad, y una prueba de estilos
// pasaría en verde con el defecto puesto.
const DIALOGOS_DEL_CATALOGO = ['src/shared/ProductEditDialog.tsx', 'src/features/admin/OptionFormDialog.tsx'];

describe('los diálogos del catálogo caben en la tableta', () => {
  for (const archivo of DIALOGOS_DEL_CATALOGO) {
    it(`${archivo} hace scroll por dentro`, () => {
      const fuente = readFileSync(archivo, 'utf8');
      const raices = fuente.match(/<DialogRoot\b[^\n]*/g) ?? [];
      expect(raices.length, 'el archivo debe tener su DialogRoot').toBeGreaterThan(0);
      for (const r of raices) expect(r).toContain('scrollBehavior="inside"');
    });
  }
});
