import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';

// CERO DIÁLOGOS DEL SISTEMA EN LA APLICACIÓN (FR-015, SC-005; caso 10 del lienzo).
//
// `confirm()`, `prompt()` y `alert()` los pinta el sistema operativo: botones de ~20 px fuera de la
// marca, que en una tableta de 7" se aciertan al revés — «Aceptar» donde se quería «Cancelar». Su
// reemplazo es `ConfirmSheet` / `ReasonSheet`. Este test recorre `src` porque una regla escrita en
// un documento no se lee a la hora de escribir un botón nuevo; un test en rojo sí.

// La API de instalación de la PWA se llama `prompt()` y no es un diálogo del sistema que se pueda
// evitar: es la única forma de ofrecer instalar.
const EXCLUIDOS = new Set([join('src', 'shared', 'pwa', 'installPrompt.ts')]);

function archivos(dir: string): string[] {
  return readdirSync(dir).flatMap((n) => {
    const p = join(dir, n);
    if (statSync(p).isDirectory()) return archivos(p);
    if (!/\.(ts|tsx)$/.test(n) || /\.test\.(ts|tsx)$/.test(n)) return [];
    return [p];
  });
}

// Quita los comentarios conservando los saltos de línea, para que el número de línea siga siendo el
// del archivo: `PrintSettingsPage.tsx` nombra `confirm()` en un comentario que explica por qué no lo
// usa, y eso no es una llamada.
function sinComentarios(src: string): string {
  return src
    .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '))
    .replace(/(^|[^:'"`])\/\/.*$/gm, (_m, antes: string) => antes);
}

const LLAMADA = /(?<![\w.$])(?:window\.)?(confirm|prompt|alert)\s*\(/g;

test('ningún archivo de la app llama a confirm, prompt o alert del navegador', () => {
  const hallazgos: string[] = [];
  for (const archivo of archivos('src')) {
    if (EXCLUIDOS.has(archivo)) continue;
    const lineas = sinComentarios(readFileSync(archivo, 'utf8')).split('\n');
    lineas.forEach((linea, i) => {
      for (const m of linea.matchAll(LLAMADA)) hallazgos.push(`${archivo}:${i + 1} ${m[1]}()`);
    });
  }
  expect(
    hallazgos,
    'usa ConfirmSheet o ReasonSheet (src/components): el diálogo del sistema no se acierta en la tableta',
  ).toEqual([]);
});
