// Compila las plantillas MJML de `emails/` al HTML que embebe el servidor
// (server/internal/mailtpl/templates/*.html.tmpl). MJML vive solo aquí, en el build: el binario no
// gana dependencias. Los includes van con `ignoreIncludes: false`: en MJML 5 vienen apagados. CI corre este script y falla si la salida versionada no coincide con la fuente.
import { readdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import mjml2html from 'mjml';

const raiz = path.resolve(import.meta.dirname, '..', '..');
const fuente = path.join(raiz, 'emails');
const destino = path.join(raiz, 'server', 'internal', 'mailtpl', 'templates');

let fallas = 0;
for (const archivo of readdirSync(fuente).filter((f) => f.endsWith('.mjml') && !f.startsWith('_')).sort()) {
  const ruta = path.join(fuente, archivo);
  const salida = await mjml2html(readFileSync(ruta, 'utf8'), {
    filePath: ruta, validationLevel: 'strict', minify: false, keepComments: false, ignoreIncludes: false,
  } as Parameters<typeof mjml2html>[1]);
  if (salida.errors.length) {
    fallas++;
    for (const e of salida.errors) console.error(`${archivo}: ${e.formattedMessage}`);
    continue;
  }
  const html = salida.html.replace(/[ \t]+$/gm, '') + (salida.html.endsWith('\n') ? '' : '\n');
  writeFileSync(path.join(destino, archivo.replace(/\.mjml$/, '.html.tmpl')), html);
  console.log(`${archivo} → ${(Buffer.byteLength(html) / 1024).toFixed(1)} KB`);
}
if (fallas) process.exit(1);
