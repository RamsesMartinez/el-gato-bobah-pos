import { describe, expect, it } from 'vitest';
import { etiquetaDeZona, zonasDelPos } from './zonas-del-pos';

// LA LEYENDA ES LO ÚNICO QUE HACE LEGIBLE LA REJILLA.
//
// La rejilla se pinta sola —está prohibido poner una captura debajo— así que 84 números sin esta
// leyenda no son un mapa. Lo que este test fija no es que las bandas sean CIERTAS (eso solo lo sabe
// una persona mirando la tableta, y por eso la leyenda va fechada), sino que sean **completas**: una
// celda sin banda es una zona que se pinta caliente y nadie puede decir qué es.

describe('las zonas del POS', () => {
  for (const [orientacion, columnas, filas] of [
    ['horizontal', 12, 7],
    ['vertical', 7, 12],
  ] as const) {
    it(`en ${orientacion} cubren las 84 celdas, sin huecos y sin traslapes`, () => {
      const cubierta = new Map<number, string>();
      for (const z of zonasDelPos(orientacion)) {
        for (let f = z.filas[0]; f <= z.filas[1]; f++) {
          for (let c = z.columnas[0]; c <= z.columnas[1]; c++) {
            const celda = f * columnas + c;
            expect(cubierta.has(celda), `la celda ${celda} está en dos bandas: "${cubierta.get(celda)}" y "${z.que}"`).toBe(false);
            cubierta.set(celda, z.que);
          }
        }
      }
      expect(cubierta.size, 'quedaron celdas sin banda: se pintan calientes y nadie sabe qué son').toBe(columnas * filas);
    });
  }

  it('cada banda se lee en una línea', () => {
    expect(etiquetaDeZona(zonasDelPos('horizontal')[0])).toMatch(/columna 0.*menú/i);
  });
});
