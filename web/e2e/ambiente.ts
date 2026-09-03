// Lo que comparten las pruebas y la limpieza: a dónde apuntan y con qué sesión.
//
// Vive aparte porque el `globalTeardown` corre FUERA de un test y no puede importar nada que
// arrastre fixtures de Playwright.
export const API = process.env.E2E_API_URL ?? 'https://api-dev.elgatobobah.com/api/v1';
export const USUARIO = process.env.E2E_USER ?? 'admin';
export const EMPRESA = process.env.E2E_SLUG ?? 'gatobobah';
export const PASSWORD = process.env.E2E_PASSWORD ?? 'Dev-ffb903b3dfb31073!';

export async function tokenDeApi(): Promise<string> {
  const r = await fetch(`${API}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: USUARIO, slug: EMPRESA, password: PASSWORD }),
  });
  if (!r.ok) throw new Error(`login del ambiente de pruebas: ${r.status}`);
  return (await r.json()).accessToken;
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
