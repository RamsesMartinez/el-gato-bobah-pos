import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';

// LAS FRONTERAS TIENEN QUE CUBRIR TODAS LAS PANTALLAS, INCLUIDAS LAS QUE NO EXISTÍAN.
//
// La regla que impide que una pantalla importe de otra vive en eslint.config.js y enumera las
// features por nombre. Esa lista explícita es a propósito —el comodín `*` de minimatch casa también
// con `..` y prohibía media aplicación— pero tiene el defecto de toda lista escrita a mano: una
// feature nueva no está en ella, así que nace SIN frontera y nadie se entera.
//
// Es exactamente el modo de fallo que estas reglas vinieron a cerrar: no un mal criterio, sino algo
// que se olvida porque nada falla al olvidarlo. Este test falla, y dice qué agregar.
test('toda feature está en la lista de fronteras de eslint', () => {
  const config = readFileSync('eslint.config.js', 'utf8');
  const carpetas = readdirSync('src/features')
    .filter((n) => statSync(join('src/features', n)).isDirectory());

  const sinFrontera = carpetas.filter((n) => !config.includes(`'../${n}/*'`));
  expect(
    sinFrontera,
    'agrega estas features a la lista de `no-restricted-imports` en eslint.config.js, o cualquier '
    + 'otra pantalla podrá importar de ellas y copiar su código sin que nada falle',
  ).toEqual([]);
});

// Y la capa de dominio no puede volverse impura por un import que se cuele: la regla de eslint lo
// impide, pero solo mientras `src/domain` siga siendo el patrón que la regla nombra. Si alguien
// mueve el dominio de carpeta, la regla deja de aplicar en silencio.
test('la capa de dominio existe donde la frontera la busca', () => {
  const config = readFileSync('eslint.config.js', 'utf8');
  expect(config).toContain("files: ['src/domain/**/*.ts']");
  expect(statSync('src/domain').isDirectory()).toBe(true);
});
