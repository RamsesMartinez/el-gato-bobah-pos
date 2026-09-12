import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

// LA MEDICIÓN VA DESPUÉS DE LA ACCIÓN, SIEMPRE.
//
// Este guardia lee el código en vez de ejecutarlo, y es a propósito: lo que hay que impedir no es
// un valor mal calculado sino una FORMA de escribir. Un `await medir…` colado en el camino del
// cobro convierte una venta en algo que espera a la red de la medición, y eso la constitución lo
// prohíbe en sus restricciones de producto — el carrito del POS funciona sin red y no puede dejar
// de hacerlo por una analítica.
//
// El test no puede probar que la promesa se cumple en todos los caminos; prueba que nadie escribió
// la forma que la rompe, que es lo que un test estático sí puede hacer y un test de runtime no
// haría nunca (habría que simular la red lenta en cada pantalla).
const ARCHIVOS = [
  'src/shared/CobrarSheet.tsx',
  'src/features/backoffice/CashPage.tsx',
  'src/app/MedidorDeUso.tsx',
  'src/api/uso.ts',
];

describe('medir nunca se espera', () => {
  it('ningún llamado a medir va con await', () => {
    const culpables: string[] = [];
    for (const archivo of ARCHIVOS) {
      const src = readFileSync(archivo, 'utf8');
      src.split('\n').forEach((linea, i) => {
        if (/await\s+(medirAccion|medirPantalla|vaciarCola)\s*\(/.test(linea)) {
          culpables.push(`${archivo}:${i + 1}: ${linea.trim()}`);
        }
      });
    }
    expect(culpables, 'esperar a la medición mete la red en el camino del operador').toEqual([]);
  });

  it('la medición del cobro ocurre dentro del onSuccess, no antes de la mutación', () => {
    const src = readFileSync('src/shared/CobrarSheet.tsx', 'utf8');
    const enSuccess = src.indexOf("medirAccion('pos', 'cobrar')");
    const success = src.indexOf('onSuccess:');
    expect(enSuccess, 'no se encontró la medición del cobro').toBeGreaterThan(-1);
    // Está después del `onSuccess:` del mismo bloque: primero se cobra, después se cuenta.
    expect(enSuccess).toBeGreaterThan(success);
  });
});
