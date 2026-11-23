// Punto de entrada del proyecto de Pages del menú (modo avanzado: `_worker.js`). Lo estático
// (robots.txt, la imagen por omisión) lo sirve Pages sin invocar esto, por `_routes.json`.
import { handle, type Env } from './handler';

declare const BUILD_ID: string;

interface Ctx {
  waitUntil(p: Promise<unknown>): void;
}

export default {
  fetch(request: Request, env: Env, ctx: Ctx): Promise<Response> {
    return handle(request, env, {
      fetch: (i, init) => fetch(i, init),
      cache: (caches as unknown as { default: Cache }).default,
      now: () => Date.now(),
      waitUntil: (p) => ctx.waitUntil(p),
      buildId: BUILD_ID,
    });
  },
};
