import { test, expect, type Browser, type Page } from '@playwright/test';
import { randomBytes } from 'node:crypto';
import { API, EMPRESA, tokenDeApi } from './ambiente';
import { pinNuevo } from './personas';

// La caja exige la asistencia de quien la abre (spec 046): los escenarios Q1, Q2, Q5, Q6, Q7 y Q8
// del quickstart, a 1024×600 con pantalla táctil.
//
// Las personas las crea la prueba (Beto cajero, Gina gerente, Caro cajera) con nombres marcados como
// de prueba y un PIN que nace aquí y no se escribe en ningún archivo; al terminar se dan de baja.
// Solo corre contra la base local: crea usuarios y abre y cierra una caja.
const LOCAL = /^https?:\/\/(localhost|127\.0\.0\.1)[:/]/.test(API);
const CAPTURAS = process.env.E2E_CAPTURAS ?? 'test-results/caja-y-asistencia';

test.use({ hasTouch: true, viewport: { width: 1024, height: 600 } });
test.describe.configure({ mode: 'serial', retries: 0 });

type Persona = { id: number; name: string; username: string; password: string; pin: string; jwt: string };

async function api<T>(jwt: string, method: string, ruta: string, body?: unknown): Promise<{ status: number; json: T }> {
  const r = await fetch(`${API}${ruta}`, {
    method, headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await r.text();
  return { status: r.status, json: (text ? JSON.parse(text) : null) as T };
}

async function ok<T>(jwt: string, method: string, ruta: string, body?: unknown): Promise<T> {
  const r = await api<T>(jwt, method, ruta, body);
  if (r.status >= 300) throw new Error(`${method} ${ruta}: ${r.status} ${JSON.stringify(r.json)}`);
  return r.json;
}

async function crearPersona(admin: string, nombre: string, role: string): Promise<Persona> {
  const sufijo = randomBytes(3).toString('hex');
  const username = `e2e-${nombre.toLowerCase()}-${sufijo}`;
  const inicial = `Ini-${randomBytes(9).toString('hex')}!`;
  const password = `Pru-${randomBytes(9).toString('hex')}!`;
  const pin = pinNuevo();
  const name = `${nombre} (prueba e2e)`;
  const u = await ok<{ id: number }>(admin, 'POST', '/users', { name, username, role, pin, password: inicial });
  // Nace con cambio de contraseña obligatorio: se cambia por la API para entrar directo a la caja.
  const login = await fetch(`${API}/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, slug: EMPRESA, password: inicial }) });
  const jwt0 = (await login.json()).accessToken as string;
  await ok(jwt0, 'POST', '/me/password', { currentPassword: inicial, newPassword: password });
  const relog = await fetch(`${API}/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, slug: EMPRESA, password }) });
  const jwt = (await relog.json()).accessToken as string;
  return { id: u.id, name, username, password, pin, jwt };
}

async function entrarComo(browser: Browser, p: Persona): Promise<Page> {
  const ctx = await browser.newContext({ hasTouch: true, viewport: { width: 1024, height: 600 } });
  const page = await ctx.newPage();
  await page.goto('/');
  await page.getByPlaceholder('usuario@empresa').fill(`${p.username}@${EMPRESA}`);
  await page.getByPlaceholder('Contraseña').fill(p.password);
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(page.getByPlaceholder('usuario@empresa')).toBeHidden({ timeout: 30_000 });
  return page;
}

async function irALaCaja(page: Page, caja: string) {
  await page.goto('/caja');
  await page.getByRole('button', { name: new RegExp(caja) }).first().click();
}

async function teclearPin(page: Page, pin: string, accion: 'Entrada' | 'Salida') {
  const hoja = page.getByRole('dialog').last();
  for (const d of pin) await hoja.getByRole('button', { name: d, exact: true }).click();
  await hoja.getByRole('button', { name: accion }).click();
}

// Cierra desde la pantalla, contando $500 en el cajón: lo que la prueba abrió.
async function cerrarDesdeLaPantalla(page: Page, avisoAlConfirmar?: string) {
  await page.getByRole('button', { name: 'Contar efectivo' }).click();
  await page.getByLabel('Piezas de $500', { exact: true }).fill('1');
  await page.getByRole('button', { name: 'Usar este conteo' }).click();
  const no = page.getByRole('button', { name: 'No, todo se queda' });
  if (await no.isVisible().catch(() => false)) await no.click();
  // La propina que heredó la caja de un turno anterior se deja para el siguiente: no es de la prueba.
  const propina = page.getByRole('button', { name: 'Se queda en caja' });
  if (await propina.isVisible().catch(() => false)) await propina.click();
  await page.getByRole('button', { name: 'Cerrar caja' }).click();
  if (avisoAlConfirmar) await expect(page.getByRole('dialog').last().getByText(avisoAlConfirmar)).toBeVisible();
  await page.getByRole('dialog').last().getByRole('button', { name: 'Cerrar caja' }).click();
  await expect(page.getByText('Caja cerrada').first()).toBeVisible({ timeout: 30_000 });
}

async function abrirPorApi(p: Persona, registerId: number) {
  await ok(p.jwt, 'POST', '/attendance/clock', { userId: p.id, pin: p.pin, expected: 'in' });
  await ok(p.jwt, 'POST', '/cash-sessions', { registerId, openingCash: 500, manualReason: 'apertura de la prueba e2e' });
}

let admin = '';
let caja = { id: 0, name: '' };
let beto: Persona, gina: Persona, caro: Persona;

test.beforeAll(async () => {
  test.skip(!LOCAL, 'crea usuarios y abre una caja: solo contra la base local');
  admin = await tokenDeApi();
  const roster = await ok<{ enabled: boolean }>(admin, 'GET', '/attendance/roster');
  test.skip(!roster.enabled, 'la empresa no tiene la asistencia encendida');
  const cajas = await ok<{ items: Array<{ id: number; name: string; openSessionId: number | null; isActive?: boolean }> }>(admin, 'GET', '/cash-registers');
  const libre = cajas.items.find((c) => c.openSessionId === null);
  test.skip(!libre, 'no hay una caja cerrada');
  caja = { id: libre!.id, name: libre!.name };
  beto = await crearPersona(admin, 'Beto', 'cajero');
  gina = await crearPersona(admin, 'Gina', 'gerente');
  caro = await crearPersona(admin, 'Caro', 'cajero');
});

test.afterAll(async () => {
  if (!admin || !gina) return;
  // Si una prueba se cayó a media caja, se cierra con autorización: un turno de prueba abierto suma
  // a «por cobrar» y bloquea el cierre de quien opera.
  const estado = await api<{ stage: string }>(gina.jwt, 'GET', `/cash-sessions/attendance?registerId=${caja.id}`);
  if (estado.json?.stage === 'close') {
    await api(gina.jwt, 'POST', '/cash-sessions/close', { registerId: caja.id, countedCash: 500,
      manualReason: 'cierre de la prueba e2e', tipsDecision: 'quedan_en_caja', authorizedClose: { reason: 'Limpieza de la prueba e2e' } });
  }
  for (const p of [beto, gina, caro]) {
    if (!p) continue;
    const role = p === gina ? 'gerente' : 'cajero';
    await api(admin, 'PATCH', `/users/${p.id}`, { name: p.name, role, isActive: false });
  }
});

test('Q1 · Q2 · abrir pide la entrada, y se marca desde la tarjeta sin volver a empezar', async ({ browser }) => {
  const page = await entrarComo(browser, beto);
  await irALaCaja(page, caja.name);
  await expect(page.getByText('Para abrir la caja, primero marca tu entrada.')).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole('button', { name: 'Contar el efectivo' })).toBeDisabled();
  await page.screenshot({ path: `${CAPTURAS}/q1-apertura.png` });

  await page.getByRole('button', { name: 'Marcar entrada' }).click();
  await expect(page.getByText(`Marcar entrada · ${beto.name}`)).toBeVisible();
  const hoja = page.getByRole('dialog').last();
  await expect(hoja.getByRole('button', { name: 'Entrada' })).toBeInViewport({ ratio: 1 });
  expect((await hoja.boundingBox())?.height ?? 0, 'la hoja de PIN no cabe en 600 px').toBeLessThanOrEqual(600);
  await page.screenshot({ path: `${CAPTURAS}/q2-hoja-de-pin.png` });
  await teclearPin(page, beto.pin, 'Entrada');

  await expect(page.getByText(/^Entrada marcada/)).toBeVisible({ timeout: 30_000 });
  const contar = page.getByRole('button', { name: 'Contar el efectivo' });
  await expect(contar).toBeEnabled();

  // Q4: se abre como hoy.
  await contar.click();
  await page.getByLabel('Piezas de $500', { exact: true }).fill('1');
  await page.getByRole('button', { name: 'Abrir caja' }).click();
  const pregunta = page.getByText('Lo contado no coincide con el cierre anterior');
  const abierta = page.getByText('Fondo inicial', { exact: true });
  await expect(pregunta.or(abierta).first()).toBeVisible({ timeout: 30_000 });
  if (await pregunta.isVisible()) {
    await page.getByRole('button', { name: 'Cambié el fondo' }).click();
    await page.getByRole('dialog').last().getByRole('button', { name: 'Abrir caja' }).click();
  }
  await expect(abierta).toBeVisible({ timeout: 30_000 });
  await page.context().close();
});

test('Q5 · Q6 · con quien abrió dentro, el aviso se ve arriba sin desplazar y el conteo espera; su salida lo libera', async ({ browser }) => {
  const page = await entrarComo(browser, gina);
  await irALaCaja(page, caja.name);
  await expect(page.getByText(`${beto.name} abrió esta caja y no ha marcado su salida`)).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole('button', { name: `Marcar salida de ${beto.name}` })).toBeVisible();
  await expect(page.getByRole('button', { name: `Cerrar sin la salida de ${beto.name}` })).toBeVisible();
  // Nada del cierre: ni la tabla, ni las notas, ni el botón.
  await expect(page.getByText('Cierre — declarado por método')).toHaveCount(0);
  await expect(page.getByPlaceholder('Notas del cierre (opcional)')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Cerrar caja' })).toHaveCount(0);
  // Se ve sin desplazar: quien llega a cerrar lo encuentra arriba, no a media página.
  await expect(page.getByText(`${beto.name} abrió esta caja y no ha marcado su salida`)).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole('button', { name: `Marcar salida de ${beto.name}` })).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: `${CAPTURAS}/q5-aviso-del-cierre.png` });

  await page.getByRole('button', { name: `Marcar salida de ${beto.name}` }).click();
  await expect(page.getByText(`Marcar salida · ${beto.name}`)).toBeVisible();
  await teclearPin(page, beto.pin, 'Salida');
  await expect(page.getByText(new RegExp(`^Salida de ${beto.name.replace(/[()]/g, '\\$&')} marcada`))).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText('Cierre — declarado por método')).toBeVisible();
  await cerrarDesdeLaPantalla(page);
  await page.context().close();
});

test('Q7 · quien abrió se fue sin marcar: Gina cierra con motivo y queda en la bitácora', async ({ browser }) => {
  await abrirPorApi(beto, caja.id);
  const page = await entrarComo(browser, gina);
  await irALaCaja(page, caja.name);
  await page.getByRole('button', { name: `Cerrar sin la salida de ${beto.name}` }).click({ timeout: 30_000 });
  const hoja = page.getByRole('dialog').last();
  await hoja.getByRole('button', { name: 'Se fue sin checar' }).click();
  await hoja.getByRole('textbox').fill('avisó por teléfono');
  await expect(hoja.getByRole('button', { name: 'Seguir con el cierre' })).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: `${CAPTURAS}/q7-autorizar.png` });
  await hoja.getByRole('button', { name: 'Seguir con el cierre' }).click();
  await expect(page.getByText(`Se cerrará sin la salida de ${beto.name}`)).toBeVisible();
  await cerrarDesdeLaPantalla(page, `La entrada de ${beto.name} quedará «sin salida».`);
  await page.context().close();

  const eventos = await ok<unknown>(admin, 'GET', '/admin/audit?action=cash.closed_without_opener_clock_out');
  expect(JSON.stringify(eventos)).toContain('Se fue sin checar: avisó por teléfono');
  const estado = await ok<{ stage: string }>(gina.jwt, 'GET', `/cash-sessions/attendance?registerId=${caja.id}`);
  expect(estado.stage).toBe('open');
});

test('Q8 · sin el permiso, el aviso dice a quién pedirlo y el servidor rechaza autorizar', async ({ browser }) => {
  await abrirPorApi(beto, caja.id);
  const page = await entrarComo(browser, caro);
  await irALaCaja(page, caja.name);
  await expect(page.getByText(`${beto.name} abrió esta caja y no ha marcado su salida`)).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText('Si ya se fue, pide a alguien con permiso que cierre la caja.')).toBeVisible();
  await expect(page.getByRole('button', { name: `Cerrar sin la salida de ${beto.name}` })).toHaveCount(0);
  await expect(page.getByText('Si ya se fue, pide a alguien con permiso que cierre la caja.')).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: `${CAPTURAS}/q8-sin-permiso.png` });
  await page.context().close();

  const r = await api(caro.jwt, 'POST', '/cash-sessions/close', { registerId: caja.id, countedCash: 500,
    manualReason: 'prueba', authorizedClose: { reason: 'Se fue sin checar' } });
  expect(r.status).toBe(403);
  // Y se deja como estaba: Gina cierra con autorización.
  await ok(gina.jwt, 'POST', '/cash-sessions/close', { registerId: caja.id, countedCash: 500,
    manualReason: 'cierre de la prueba e2e', tipsDecision: 'quedan_en_caja', authorizedClose: { reason: 'Se fue sin checar' } });
});
