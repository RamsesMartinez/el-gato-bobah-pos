import { beforeEach, describe, expect, it } from 'vitest';
import { handle, type Deps, type Env, FRESCO_MS, GUARDADO_MS } from './handler';
import type { PublicMenu } from '../plantilla/datos';

const ENV: Env = { API_ORIGIN: 'https://api.test', MEDIA_ORIGIN: 'https://media.test', MENU_EDGE_KEY: 's'.repeat(32) };

const MENU: PublicMenu = {
  kind: 'menu', name: 'fonda-lupe', qrPath: '/q/0123456789ab', business: { name: 'Fonda Lupe' },
  brand: { palette: 'brand' }, template: 'classic',
  categories: [{ id: 1, name: 'Café', family: 'coffee', imageUrl: null, subcategories: [],
    products: [{ id: 9, name: 'Americano', description: null, price: '30', imageUrl: null, soldOut: false }] }],
};

class FakeCache {
  store = new Map<string, Response>();
  async match(req: Request) { const r = this.store.get(req.url); return r ? r.clone() : undefined; }
  async put(req: Request, res: Response) { this.store.set(req.url, res.clone()); }
}

let calls: Request[];
let api: (req: Request) => Promise<Response>;
let cache: FakeCache;
let now: number;
const pending: Promise<unknown>[] = [];

function deps(): Deps {
  return {
    fetch: async (input: RequestInfo | URL, init?: RequestInit) => {
      const req = new Request(input, init);
      calls.push(req);
      return api(req);
    },
    cache, now: () => now, waitUntil: (p) => { pending.push(p); }, buildId: 'b1',
  };
}
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const get = async (path: string, headers: Record<string, string> = {}) => {
  const r = await handle(new Request(`https://menu.test${path}`, { headers }), ENV, deps());
  await Promise.all(pending.splice(0));
  return r;
};

beforeEach(() => {
  calls = []; cache = new FakeCache(); now = 1_000_000;
  api = async () => json(200, MENU);
});

describe('el menú en el borde', () => {
  it('sirve el HTML del menú, sin scripts y con la CSP de medios', async () => {
    const r = await get('/fonda-lupe', { 'CF-Connecting-IP': '201.1.1.1' });
    expect(r.status).toBe(200);
    expect(r.headers.get('Content-Type')).toContain('text/html');
    const csp = r.headers.get('Content-Security-Policy')!;
    expect(csp).toContain("default-src 'none'");
    expect(csp).toMatch(/img-src 'self' data: [^;]*https:\/\/media\.test/);
    expect(csp).not.toContain('script-src');
    expect(r.headers.get('Cache-Control')).toBe('public, max-age=60');
    const html = await r.text();
    expect(html).toContain('Americano');
    expect(html).not.toMatch(/<script/i);
    expect(html).toContain('<link rel="canonical" href="https://menu.test/fonda-lupe">');
  });

  it('le dice a la API a quién atiende, con el secreto', async () => {
    await get('/fonda-lupe', { 'CF-Connecting-IP': '201.1.1.1' });
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe('https://api.test/api/v1/public/menus/fonda-lupe');
    expect(calls[0].headers.get('X-Menu-Edge-Key')).toBe(ENV.MENU_EDGE_KEY);
    expect(calls[0].headers.get('X-Menu-Client-IP')).toBe('201.1.1.1');
  });

  it('una forma imposible responde 404 sin gastar una petición a la API', async () => {
    for (const p of ['/ab', '/a_b', '/x/y', '/' + 'a'.repeat(41), '/%00']) {
      const r = await get(p);
      expect(r.status, p).toBe(404);
    }
    expect(calls).toHaveLength(0);
  });

  it('mayúsculas o diagonal final llevan a la forma canónica', async () => {
    let r = await get('/Fonda-Lupe');
    expect(r.status).toBe(302);
    expect(r.headers.get('Location')).toBe('/fonda-lupe');
    r = await get('/fonda-lupe/');
    expect(r.headers.get('Location')).toBe('/fonda-lupe');
    expect(calls).toHaveLength(0);
  });

  it('un nombre anterior y un QR llevan al vigente con 302, nunca 301', async () => {
    api = async (req) => json(200, { kind: 'moved', name: req.url.includes('/q/') ? 'fonda-lupe' : 'fonda-lupe' });
    let r = await get('/nombre-viejo');
    expect(r.status).toBe(302);
    expect(r.headers.get('Location')).toBe('/fonda-lupe');
    expect(r.headers.get('Cache-Control')).toBe('public, max-age=300');
    r = await get('/q/0123456789ab');
    expect(r.status).toBe(302);
    expect(calls.at(-1)!.url).toBe('https://api.test/api/v1/public/menus/q/0123456789ab');
  });

  it('un nombre que no existe es 404 y también se guarda', async () => {
    api = async () => json(404, { code: 'NOT_FOUND' });
    expect((await get('/no-existe')).status).toBe(404);
    expect((await get('/no-existe')).status).toBe(404);
    expect(calls).toHaveLength(1);
  });

  it('la query string no crea copias nuevas', async () => {
    await get('/fonda-lupe?a=1');
    await get('/fonda-lupe?a=2');
    expect(calls).toHaveLength(1);
  });

  it('fresco no va a la API; vencido sí', async () => {
    await get('/fonda-lupe');
    now += FRESCO_MS - 1;
    await get('/fonda-lupe');
    expect(calls).toHaveLength(1);
    now += 2;
    await get('/fonda-lupe');
    expect(calls).toHaveLength(2);
  });

  it('con la API caída sirve la última copia buena de hasta 24 h', async () => {
    await get('/fonda-lupe');
    api = async () => json(502, {});
    now += FRESCO_MS + 1;
    let r = await get('/fonda-lupe');
    expect(r.status).toBe(200);
    expect(await r.text()).toContain('Americano');
    api = async () => { throw new Error('red'); };
    r = await get('/fonda-lupe');
    expect(r.status).toBe(200);
    now += GUARDADO_MS;
    r = await get('/fonda-lupe');
    expect(r.status).toBe(503);
    expect(r.headers.get('Retry-After')).toBe('30');
  });

  it('sin copia y con la API caída: 503 con texto para quien abrió el enlace', async () => {
    api = async () => json(429, {});
    const r = await get('/fonda-lupe');
    expect(r.status).toBe(503);
    expect(await r.text()).toContain('No pudimos cargar el menú');
  });

  it('una plantilla nueva no espera a que venza la copia vieja', async () => {
    await get('/fonda-lupe');
    const d = { ...deps(), buildId: 'b2' };
    await handle(new Request('https://menu.test/fonda-lupe'), ENV, d);
    expect(calls).toHaveLength(2);
  });

  it('la portada no lista negocios', async () => {
    const r = await get('/');
    expect(r.status).toBe(200);
    expect(calls).toHaveLength(0);
  });
});
