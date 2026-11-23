// La Function del menú público (spec 097): pide el menú a la API, lo pinta con la plantilla y lo
// guarda en el borde. Las dependencias (fetch, caché, reloj) entran por parámetro para probarla sin
// Cloudflare; worker.ts la conecta con las de verdad.
import type { PublicMenu, PublicMoved } from '../plantilla/datos';
import { paginaSimple, renderMenu } from '../plantilla';

export interface Env {
  API_ORIGIN: string;
  /** Un origen más para las fotos en la CSP; las que trae el menú se permiten solas. */
  MEDIA_ORIGIN?: string;
  /** El secreto con el que la API cree la IP del visitante que se le reenvía. */
  MENU_EDGE_KEY?: string;
}

export interface CacheLike {
  match(req: Request): Promise<Response | undefined>;
  put(req: Request, res: Response): Promise<void>;
}

export interface Deps {
  fetch: typeof fetch;
  cache: CacheLike;
  now: () => number;
  waitUntil: (p: Promise<unknown>) => void;
  /** Va en la llave de la caché: la Cache API sobrevive a los despliegues, y sin él una plantilla
   *  nueva tardaría hasta que venciera la copia vieja. */
  buildId: string;
}

/** Un cambio del catálogo o un agotado se ve en el borde a lo más en este tiempo (SC-006). */
export const FRESCO_MS = 60_000;
/** Si la API no responde, se sirve la última copia buena de hasta este tiempo (FR-024). */
export const GUARDADO_MS = 24 * 60 * 60_000;

const NOMBRE = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const TOKEN = /^[0-9a-f]{12}$/;
const GUARDADO_EN = 'X-Menu-Stored-At';

const html = (body: string, status: number, extra: Record<string, string> = {}) =>
  new Response(body, { status, headers: { 'Content-Type': 'text/html; charset=utf-8', ...extra } });

const redirige = (to: string, maxAge: number) =>
  new Response(null, { status: 302, headers: { Location: to, 'Cache-Control': `public, max-age=${maxAge}` } });

function seguridad(r: Response, imgOrigins: string[]): Response {
  const h = new Headers(r.headers);
  h.set('Content-Security-Policy',
    `default-src 'none'; img-src 'self' data: ${imgOrigins.join(' ')}; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`.replace(/ ;/g, ';'));
  h.set('X-Content-Type-Options', 'nosniff');
  h.set('Referrer-Policy', 'strict-origin-when-cross-origin');
  return new Response(r.body, { status: r.status, headers: h });
}

const noExiste = () => html(paginaSimple('Este menú no existe', 'Revisa la dirección o pídele el enlace al negocio.'), 404, { 'Cache-Control': 'public, max-age=60' });
const noCargo = () => html(paginaSimple('No pudimos cargar el menú', 'Intenta de nuevo en unos segundos.'), 503, { 'Cache-Control': 'no-store', 'Retry-After': '30' });

export async function handle(request: Request, env: Env, d: Deps): Promise<Response> {
  const url = new URL(request.url);
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    return new Response(null, { status: 405, headers: { Allow: 'GET, HEAD' } });
  }
  let path: string;
  try {
    path = decodeURIComponent(url.pathname);
  } catch {
    return seguridad(noExiste(), []);
  }
  if (path === '/') {
    return seguridad(html(paginaSimple('Menús en línea', 'Abre el enlace o escanea el código QR que te compartió el negocio.'), 200, { 'Cache-Control': 'public, max-age=3600' }), []);
  }

  const partes = path.split('/').filter(Boolean);
  // /q/<código>: el QR impreso.
  if (partes.length === 2 && partes[0] === 'q') {
    if (!TOKEN.test(partes[1])) return seguridad(noExiste(), []);
    return conCache(`/q/${partes[1]}`, `/api/v1/public/menus/q/${partes[1]}`, request, url, env, d);
  }
  if (partes.length !== 1) return seguridad(noExiste(), []);

  // La forma canónica: minúsculas y sin diagonal final. Un enlace dictado por teléfono o escrito en
  // un volante llega como sea.
  const nombre = partes[0].toLowerCase();
  if (nombre !== partes[0] || path.endsWith('/')) {
    if (NOMBRE.test(nombre) && nombre.length >= 3 && nombre.length <= 40) return redirige(`/${nombre}`, 300);
    return seguridad(noExiste(), []);
  }
  // Una forma imposible no gasta una petición a la API.
  if (!NOMBRE.test(nombre) || nombre.length < 3 || nombre.length > 40) return seguridad(noExiste(), []);
  return conCache(`/${nombre}`, `/api/v1/public/menus/${nombre}`, request, url, env, d);
}

async function conCache(ruta: string, apiPath: string, request: Request, url: URL, env: Env, d: Deps): Promise<Response> {
  // La llave ignora la query string a propósito: variarla no fuerza peticiones a la API.
  const llave = new Request(`${url.origin}/__cache/${d.buildId}${ruta}`);
  const guardada = await d.cache.match(llave);
  const edad = guardada ? d.now() - Number(guardada.headers.get(GUARDADO_EN) ?? 0) : Infinity;
  if (guardada && edad < FRESCO_MS) return limpia(guardada);

  let fresca: Response | null;
  try {
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (env.MENU_EDGE_KEY) {
      headers['X-Menu-Edge-Key'] = env.MENU_EDGE_KEY;
      const ip = request.headers.get('CF-Connecting-IP');
      if (ip) headers['X-Menu-Client-IP'] = ip;
    }
    const r = await d.fetch(`${env.API_ORIGIN}${apiPath}`, { headers });
    fresca = await responder(r, ruta, url, env);
  } catch {
    fresca = null;
  }

  if (!fresca) {
    if (guardada && edad < GUARDADO_MS) return limpia(guardada);
    return seguridad(noCargo(), []);
  }
  const copia = fresca.clone();
  const h = new Headers(copia.headers);
  h.set(GUARDADO_EN, String(d.now()));
  // La Cache API guarda según Cache-Control: la copia vive 24 h aunque al cliente se le diga 60 s.
  h.set('Cache-Control', `public, max-age=${GUARDADO_MS / 1000}`);
  d.waitUntil(d.cache.put(llave, new Response(copia.body, { status: copia.status, headers: h })));
  return fresca;
}

/** null = la API no está bien (5xx, 429, red): se intenta la copia guardada. */
async function responder(r: Response, ruta: string, url: URL, env: Env): Promise<Response | null> {
  if (r.status === 404) return seguridad(noExiste(), []);
  if (!r.ok) return null;
  const body = (await r.json()) as PublicMenu | PublicMoved;
  if (body.kind === 'moved') {
    if (!NOMBRE.test(body.name)) return null;
    return redirige(`/${body.name}`, 300);
  }
  if (ruta.startsWith('/q/')) return null;
  const origins = new Set<string>(env.MEDIA_ORIGIN ? [env.MEDIA_ORIGIN] : []);
  for (const c of body.categories) {
    for (const u of [c.imageUrl, ...c.products.map((p) => p.imageUrl), ...c.subcategories.flatMap((s) => s.products.map((p) => p.imageUrl))]) {
      if (u) origins.add(new URL(u).origin);
    }
  }
  const page = renderMenu(body, { canonicalUrl: `${url.origin}/${body.name}`, ogDefaultImage: `${url.origin}/og-default.png` });
  return seguridad(html(page, 200, { 'Cache-Control': 'public, max-age=60' }), [...origins]);
}

/** Lo que se guardó sale sin la marca interna de cuándo se guardó y con la vida que ve el cliente. */
function limpia(r: Response): Response {
  const h = new Headers(r.headers);
  h.delete(GUARDADO_EN);
  if (r.status === 200 || r.status === 404) h.set('Cache-Control', 'public, max-age=60');
  if (r.status === 302) h.set('Cache-Control', 'public, max-age=300');
  return new Response(r.body, { status: r.status, headers: h });
}
