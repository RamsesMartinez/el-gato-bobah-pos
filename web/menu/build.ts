// Construye el proyecto de Pages del menú público en dist-menu/: lo estático de menu/public y la
// Function empaquetada como _worker.js. El id del build va en la llave de la caché del borde.
import { cpSync, rmSync } from 'node:fs';

const out = 'dist-menu';
rmSync(out, { recursive: true, force: true });
cpSync('menu/public', out, { recursive: true });
const buildId = process.env.GITHUB_SHA ? process.env.GITHUB_SHA.slice(0, 12) : `local-${Date.now()}`;
const r = await Bun.build({
  entrypoints: ['menu/borde/worker.ts'],
  outdir: out,
  naming: '_worker.js',
  target: 'browser',
  format: 'esm',
  minify: true,
  define: { BUILD_ID: JSON.stringify(buildId) },
});
if (!r.success) {
  for (const l of r.logs) console.error(l);
  process.exit(1);
}
console.log(`menú público en ${out}/ (build ${buildId})`);
