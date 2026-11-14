import { test, expect, type Browser, type Page, type APIRequestContext } from '@playwright/test';
import { randomBytes } from 'node:crypto';
import { API, EMPRESA, tokenDeRequest } from './ambiente';
import { altaDePrueba, bajaDePrueba, type PersonaDePrueba } from './personas';
import { ALTO, ALTO_2K, ANCHO, ANCHO_2K } from './pantalla';
import { iniciarSesion } from './sesion';

// NOTIFICACIONES DE INVENTARIO (spec 084, T052): un cajero avisa «se acabó», la gerente lo ve en la
// campana sin recargar, le llega el correo y arma la compra. Contra el `make start` del worktree, a
// 1280×854 y en 2K. Todo lo que se crea se deshace: el insumo de prueba se archiva (eso cierra sus
// avisos) y las dos personas se dan de baja.
//
// El correo de avisos sale por bloques de 5 minutos y el proceso revisa cada minuto: la prueba que
// lo espera puede tardar hasta ~7 minutos, a propósito. Bajar el bloque para la prueba sería probar
// otra cosa.

const CAPTURAS = process.env.E2E_CAPTURAS ?? 'test-results/notificaciones';
// mailpit del worktree (8200+NN). Sin él, las pruebas de correo se saltan y lo dicen.
const MAILPIT = process.env.E2E_MAILPIT_URL ?? '';

test.describe.configure({ mode: 'serial', retries: 0 });

let jwt = '';
let insumo: { id: number; name: string } | null = null;
let cajero: PersonaDePrueba | null = null;
let gerente: PersonaDePrueba | null = null;
const correoGerente = `gerente-${randomBytes(3).toString('hex')}@ejemplo.mx`;

async function api(request: APIRequestContext, method: 'get' | 'post' | 'put' | 'patch', path: string, data?: unknown, token = jwt) {
  const r = await request[method](`${API}${path}`, { headers: { Authorization: `Bearer ${token}` }, data });
  if (!r.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${r.status()} ${await r.text()}`);
  const t = await r.text();
  return t ? JSON.parse(t) : null;
}

// conContrasenaPropia: la persona nace con cambio obligatorio; se cambia por API para que el
// navegador entre directo, igual que alguien que ya entró una vez.
async function conContrasenaPropia(request: APIRequestContext, p: PersonaDePrueba) {
  const r = await request.post(`${API}/auth/login`, { data: { username: p.username, slug: EMPRESA, password: p.password } });
  if (!r.ok()) throw new Error(`login de ${p.username}: ${r.status()}`);
  const token = (await r.json()).accessToken as string;
  const nueva = `Nva-${randomBytes(9).toString('hex')}!`;
  await api(request, 'post', '/me/password', { currentPassword: p.password, newPassword: nueva }, token);
  p.password = nueva;
}

async function entrarComo(browser: Browser, p: PersonaDePrueba, ruta: string, viewport = { width: ANCHO, height: ALTO }) {
  const ctx = await browser.newContext({ viewport, hasTouch: true });
  const page = await ctx.newPage();
  await page.goto(ruta);
  await page.getByPlaceholder('usuario@empresa').fill(`${p.username}@${EMPRESA}`);
  await page.getByPlaceholder('Contraseña').fill(p.password);
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(page.getByPlaceholder('usuario@empresa')).toBeHidden({ timeout: 20_000 });
  // La entrada lleva a la pantalla de inicio; de ahí se va a la que se quiere, como quien toca el riel.
  await page.goto(ruta);
  await page.waitForLoadState('networkidle');
  return page;
}

const campana = (page: Page) => page.getByRole('button', { name: /^Avisos, \d+ sin leer$/ });
async function sinLeer(page: Page) {
  const label = (await campana(page).getAttribute('aria-label')) ?? '';
  return Number(/(\d+)/.exec(label)?.[1] ?? NaN);
}

async function sinDesbordeHorizontal(page: Page) {
  const { scroll, client } = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, client: document.documentElement.clientWidth }));
  expect(scroll, 'la página se desborda a lo ancho').toBeLessThanOrEqual(client);
}

async function correos(): Promise<{ To: { Address: string }[]; Subject: string; ID: string }[]> {
  const r = await fetch(`${MAILPIT}/api/v1/messages?limit=50`);
  return ((await r.json()).messages ?? []) as { To: { Address: string }[]; Subject: string; ID: string }[];
}
async function cuerpo(id: string) {
  return ((await (await fetch(`${MAILPIT}/api/v1/message/${id}`)).json()).HTML ?? '') as string;
}

test.beforeAll(async ({ request }) => {
  jwt = await tokenDeRequest(request);
  const unidades: { id: number; code: string }[] = (await api(request, 'get', '/units')).items;
  const pieza = unidades.find((u) => u.code === 'pieza');
  if (!pieza) throw new Error('la base no tiene la unidad pieza');
  const name = `E2E Vasos 16 oz ${randomBytes(2).toString('hex')}`;
  const creado = await api(request, 'post', '/stock/articles', { name, baseUnitId: pieza.id, initialStock: '40' });
  insumo = { id: creado.id, name };
  await api(request, 'put', `/stock/articles/${insumo.id}/low-stock`, { enabled: true, minStock: '10', onMin: true, onOut: true, banner: true, coverageDays: 14, branchMinStock: null });

  cajero = await altaDePrueba(jwt, 'Cajero', 'cajero');
  gerente = await altaDePrueba(jwt, 'Gerente', 'gerente');
  await api(request, 'patch', `/users/${gerente.id}`, { name: gerente.name, role: gerente.role, isActive: true, recoveryEmail: correoGerente });
  await conContrasenaPropia(request, cajero);
  await conContrasenaPropia(request, gerente);

  const dest = await api(request, 'get', '/settings/notification-recipients');
  const ella = dest.people.find((p: { userId: number }) => p.userId === gerente!.id);
  if (!ella?.stock || !ella.hasEmail) throw new Error(`la gerente no recibe por omisión: ${JSON.stringify(ella)}`);
  if (MAILPIT) await fetch(`${MAILPIT}/api/v1/messages`, { method: 'DELETE' });
  console.log(`[e2e] insumo ${insumo.id} «${insumo.name}» · cajero ${cajero.id} · gerente ${gerente.id}`);
});

test.afterAll(async ({ request }) => {
  jwt = await tokenDeRequest(request);
  if (insumo) await api(request, 'post', `/stock/articles/${insumo.id}/archive`, { confirmOnHand: true }).catch((e) => console.log(`[e2e] archivar ${insumo!.id}: ${e}`));
  for (const p of [cajero, gerente]) if (p) await bajaDePrueba(jwt, p);
  console.log(`[e2e] limpieza: insumo ${insumo?.id} archivado, personas ${cajero?.id} y ${gerente?.id} de baja`);
});

test('el cajero avisa «se acabó» y la gerente lo ve en la campana sin recargar', async ({ browser }) => {
  const g = await entrarComo(browser, gerente!, '/almacen');
  await expect(campana(g)).toBeVisible();
  // La campana vive al pie del riel: a 854 de alto tiene que caber sin desplazar.
  const caja = await campana(g).boundingBox();
  expect(caja!.y + caja!.height, 'la campana queda fuera de la pantalla').toBeLessThanOrEqual(ALTO);
  expect(caja!.height).toBeGreaterThanOrEqual(44);
  const antes = await sinLeer(g);

  const c = await entrarComo(browser, cajero!, '/se-acabo');
  await c.getByLabel('¿Qué se acabó?').fill(insumo!.name);
  await expect(c.getByText('1 coinciden')).toBeVisible();
  // «No lo encuentro» también lleva el texto buscado: se excluye.
  const mosaico = c.getByRole('button', { name: new RegExp(insumo!.name) }).filter({ hasNotText: 'No lo encuentro' });
  await mosaico.click();
  await expect(c.getByText('Toca otra vez para avisar que se acabó')).toBeVisible();
  await c.waitForTimeout(500); // pasada la gracia del doble toque
  await mosaico.click();
  const t0 = Date.now();
  await expect(c.getByRole('status').filter({ hasText: 'quedó en 0' })).toBeVisible();
  await sinDesbordeHorizontal(c);
  await c.screenshot({ path: `${CAPTURAS}/1280-cajero-se-acabo.png` });
  // Quien reporta no recibe la franja de su propio aviso.
  await expect(c.getByTestId('franja-de-agotado')).toHaveCount(0);

  // Sube en dos: de 40 a 0 cruza el mínimo de 10 («stock bajo») y se acaba («se acabó»), y son dos
  // eventos que se eligen por separado en la ficha.
  await expect.poll(() => sinLeer(g), { timeout: 5_000, intervals: [100] }).toBe(antes + 2);
  const ms = Date.now() - t0;
  console.log(`[e2e] SC-001: la campana de la gerente subió ${ms} ms después del aviso (sin recargar)`);
  expect(ms).toBeLessThan(5_000);
  await expect(g.getByTestId('franja-de-agotado')).toContainText(insumo!.name);
  await g.screenshot({ path: `${CAPTURAS}/1280-gerente-franja.png` });

  await campana(g).click();
  const panel = g.getByTestId('panel-de-avisos');
  await expect(panel.getByText(insumo!.name)).toHaveCount(2);
  await expect(panel.getByText(/Lo reportó Cajero \(prueba e2e\)/)).toBeVisible();
  await g.screenshot({ path: `${CAPTURAS}/1280-gerente-panel.png` });
  // Con el panel abierto el resto de la página queda oculto para lectores de pantalla: se cierra
  // para leer la campana.
  await panel.getByRole('button', { name: 'Cerrar' }).click();
  await expect.poll(() => sinLeer(g)).toBe(antes);
  await c.context().close();
  await g.context().close();
});

test('la gerente arma la compra y se la manda por correo', async ({ browser }) => {
  const g = await entrarComo(browser, gerente!, '/almacen?tab=compra');
  const renglon = g.getByRole('tabpanel', { name: 'Lista de compra' }).getByText(insumo!.name, { exact: true });
  await expect(renglon).toBeVisible();
  await sinDesbordeHorizontal(g);
  await g.screenshot({ path: `${CAPTURAS}/1280-lista-de-compra.png` });
  test.skip(!MAILPIT, 'sin E2E_MAILPIT_URL no se revisa el correo');
  await g.getByRole('button', { name: 'Enviarme por correo' }).click();
  await expect.poll(async () => (await correos()).filter((m) => m.To.some((t) => t.Address === correoGerente) && /compra/i.test(m.Subject)).length, { timeout: 15_000 }).toBe(1);
  await g.context().close();
});

test('el correo de avisos llega una vez al cerrar el bloque de 5 minutos', async () => {
  test.skip(!MAILPIT, 'sin E2E_MAILPIT_URL no se revisa el correo');
  test.setTimeout(9 * 60_000);
  const deAvisos = async () => (await correos()).filter((m) => m.To.some((t) => t.Address === correoGerente) && m.Subject.startsWith('Avisos de inventario'));
  await expect.poll(async () => (await deAvisos()).length, { timeout: 8 * 60_000, intervals: [10_000] }).toBeGreaterThanOrEqual(1);
  const enviados = await deAvisos();
  expect(enviados, 'el mismo bloque mandó más de un correo').toHaveLength(1);
  const [m] = enviados;
  expect(await cuerpo(m.ID)).toContain(insumo!.name);
  console.log(`[e2e] correo de avisos: «${m.Subject}»`);
});

test('en 2K nada se estira ni se desborda', async ({ browser }) => {
  const viewport = { width: ANCHO_2K, height: ALTO_2K };
  const c = await entrarComo(browser, cajero!, '/se-acabo', viewport);
  await expect(c.getByLabel('¿Qué se acabó?')).toBeVisible();
  await sinDesbordeHorizontal(c);
  await c.screenshot({ path: `${CAPTURAS}/2k-se-acabo.png` });
  const g = await entrarComo(browser, gerente!, '/almacen?tab=compra', viewport);
  await campana(g).click();
  await expect(g.getByTestId('panel-de-avisos')).toBeVisible();
  const ancho = (await g.getByTestId('panel-de-avisos').boundingBox())!.width;
  expect(ancho, 'el panel se estira con la pantalla').toBeLessThanOrEqual(600);
  await sinDesbordeHorizontal(g);
  await g.screenshot({ path: `${CAPTURAS}/2k-gerente-panel.png` });
  await c.context().close();
  await g.context().close();
});

// La campana vive al pie del riel y le quita alto a la lista de opciones: este número se fija para
// que una pieza nueva al pie (o un renglón más alto) no se coma otra opción sin que nadie lo decida.
// Medido el 2026-11-14 con la campana ya puesta: 11 opciones enteras a 1280×854.
const VISIBLES_DEL_ADMIN = 11;
test('a 1280×854 el admin ve las mismas opciones del riel sin desplazar', async ({ page }) => {
  await page.goto('/');
  await iniciarSesion(page);
  await expect(campana(page)).toBeVisible();
  const tope = (await campana(page).boundingBox())!.y;
  // Las opciones del riel son los enlaces dentro de sus 76 px.
  const enteras = await page.locator('a[href^="/"]').evaluateAll((els, limite) =>
    els.filter((e) => { const r = e.getBoundingClientRect(); return r.height > 0 && r.right <= 76 && r.top >= 0 && r.bottom <= limite; }).length, tope);
  console.log(`[e2e] opciones del riel enteras a la vista: ${enteras}`);
  expect(enteras).toBeGreaterThanOrEqual(VISIBLES_DEL_ADMIN);
});
