import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, sep } from 'node:path';

// EL FORMATEO DE FECHAS VIVE EN UN SOLO LUGAR, Y ESTE TEST LO SOSTIENE.
//
// Once pantallas llamaban a `toLocaleString` por su cuenta, sin zona, y por eso todas decían la hora
// del navegador. Migrarlas una vez no sirve de nada si la pantalla número doce vuelve a hacerlo: sin
// este guardia, el arreglo de hoy se deshace solo en tres meses y nadie lo nota hasta que un ticket
// dice una hora que no fue.
//
// El helper se recorta antes de buscar: ahí llamar a `toLocale*` es su trabajo.
const PERMITIDOS = ['src/utils/horaDelNegocio.ts'];

// El dinero se formatea con `toLocaleString` sobre un NÚMERO y no lleva zona; es otro problema.
const ES_DE_DINERO = /\bn\.toLocaleString\(/;

function archivos(dir: string): string[] {
  return readdirSync(dir).flatMap((n) => {
    const p = join(dir, n);
    if (statSync(p).isDirectory()) return archivos(p);
    return /\.tsx?$/.test(n) && !/\.test\.tsx?$/.test(n) ? [p] : [];
  });
}

test('ninguna pantalla formatea fechas por su cuenta', () => {
  const culpables: string[] = [];
  for (const p of archivos('src')) {
    const rel = p.split(sep).join('/');
    if (PERMITIDOS.some((ok) => rel.endsWith(ok))) continue;
    const src = readFileSync(p, 'utf8');
    for (const linea of src.split('\n')) {
      if (!/toLocale(String|TimeString|DateString)\(/.test(linea)) continue;
      if (ES_DE_DINERO.test(linea)) continue;
      culpables.push(`${rel}: ${linea.trim()}`);
    }
  }
  expect(culpables, 'formatear fechas fuera de horaDelNegocio.ts las pinta en la zona del navegador').toEqual([]);
});
