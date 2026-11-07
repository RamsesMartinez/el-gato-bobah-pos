import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';

// Roles por empresa (spec 068, constitución «Roles: modelo híbrido»): el código pregunta por
// PERMISO, nunca por el nombre de un rol. Un rol propio de una empresa no se llama «admin» ni
// «gerente», y una comparación contra el nombre lo deja sin pantallas que su rol sí permite —o con
// botones que el servidor rebota con 403—. La consola es otro producto y sus roles son otros.
const SRC = resolve(__dirname, '..');
const PROHIBIDO = /\brole\)?\s*!?==|[!=]==?\s*(?:\w+\.)*role\b|\bcanAccess\(/;

function fuentes(dir: string): string[] {
  return readdirSync(dir).flatMap((nombre) => {
    const ruta = join(dir, nombre);
    if (statSync(ruta).isDirectory()) return nombre === 'consola' && dir === SRC ? [] : fuentes(ruta);
    if (!/\.(ts|tsx)$/.test(nombre) || /\.test\.(ts|tsx)$/.test(nombre)) return [];
    return [ruta];
  });
}

describe('nombre de rol en el front', () => {
  it('ninguna pantalla compara el nombre del rol', () => {
    const hallazgos = fuentes(SRC).flatMap((archivo) =>
      readFileSync(archivo, 'utf8').split('\n')
        .map((linea, i) => ({ linea, i }))
        .filter(({ linea }) => PROHIBIDO.test(linea))
        .map(({ linea, i }) => `${relative(SRC, archivo)}:${i + 1}: ${linea.trim()}`));
    expect(hallazgos, 'usa can(permiso, user) de app/permissions.ts').toEqual([]);
  });
});
