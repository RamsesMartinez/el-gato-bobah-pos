import type { APIRequestContext } from '@playwright/test';

// Lo que comparten las pruebas y la limpieza: a dónde apuntan y con qué sesión.
//
// Vive aparte porque el `globalTeardown` corre FUERA de un test y no puede importar nada que
// arrastre fixtures de Playwright.
export const API = process.env.E2E_API_URL ?? 'https://api-dev.elgatobobah.com/api/v1';
export const USUARIO = process.env.E2E_USER ?? 'admin';
export const EMPRESA = process.env.E2E_SLUG ?? 'gatobobah';
export const PASSWORD = process.env.E2E_PASSWORD ?? 'Dev-ffb903b3dfb31073!';

// EL TOKEN SE REUSA DENTRO DE LA MISMA CORRIDA.
//
// `/auth` tiene un limitador de 60 peticiones por minuto y por IP, y la suite lo estaba tumbando
// sola: medido contra el ambiente de pruebas, una corrida dejó **8 respuestas 429**, y el síntoma
// que se ve es un test que espera 30 segundos a una pantalla que nunca entra —intermitente, y que
// pasa al reintentar—. Un gate que falla una vez de cada tantas enseña a no creerle.
//
// Se reusa por 5 minutos y no para siempre: el token caduca, y un 401 a media suite se leería como
// un defecto del servidor.
let enCache: { jwt: string; hasta: number } | null = null;

export async function tokenDeApi(): Promise<string> {
  if (enCache && Date.now() < enCache.hasta) return enCache.jwt;
  const r = await fetch(`${API}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: USUARIO, slug: EMPRESA, password: PASSWORD }),
  });
  if (!r.ok) throw new Error(`login del ambiente de pruebas: ${r.status}`);
  const jwt = (await r.json()).accessToken as string;
  enCache = { jwt, hasta: Date.now() + 5 * 60_000 };
  return jwt;
}

// tokenDeRequest: el mismo token, pedido con el `request` de Playwright.
//
// Existe para que los specs no se traigan cada uno su copia del login —eran cinco, idénticas— y
// sobre todo para que compartan el caché de arriba: contadas, esas copias hacían la mayoría de las
// peticiones que tumbaban el limitador. El `import type` se borra al compilar, así que esto no
// arrastra fixtures de Playwright al `globalTeardown`.
export async function tokenDeRequest(request: APIRequestContext): Promise<string> {
  if (enCache && Date.now() < enCache.hasta) return enCache.jwt;
  const r = await request.post(`${API}/auth/login`, {
    data: { username: USUARIO, slug: EMPRESA, password: PASSWORD },
  });
  if (!r.ok()) throw new Error(`login del ambiente de pruebas: ${r.status()}`);
  const jwt = (await r.json()).accessToken as string;
  enCache = { jwt, hasta: Date.now() + 5 * 60_000 };
  return jwt;
}

export async function pedidosEnCurso(jwt: string) {
  const r = await fetch(`${API}/orders/open`, { headers: { Authorization: `Bearer ${jwt}` } });
  if (!r.ok) throw new Error(`/orders/open: ${r.status}`);
  const body = await r.json();
  return (body.items ?? []) as Array<{
    id: number; number: number; folioName: string; outstanding: string;
    enPreparacion: boolean; deliveryPlatformId: number | null;
  }>;
}

// Dónde se anotan los pedidos que ya estaban abiertos cuando empezó la suite.
export const MARCA = process.env.E2E_MARCA ?? '.playwright-abiertos.json';
