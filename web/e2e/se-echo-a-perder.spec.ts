import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
import { API, tokenDeRequest } from './ambiente';
import { entrar } from './pos';

// SE ECHÓ A PERDER (spec 039, T040): la captura, la franja de por aprobar y la pestaña de Almacén
// medidas a 1024×600 contra el ambiente de pruebas. El ambiente lo usa una persona: todo lo que se
// registra se deshace o se rechaza, el tope vuelve a como estaba y los motivos creados se archivan.

const CAPTURAS = process.env.E2E_CAPTURAS ?? 'test-results/se-echo-a-perder';

test.describe.configure({ mode: 'serial', retries: 0 });

let jwt = '';
let topeOriginal: string | null = null;
const motivosCreados: { id: number; name: string; description: string }[] = [];
const registrados: number[] = [];
let insumo: { id: number; name: string; packs: { id: number; name: string }[] } | null = null;
// El empaque temporal: dev no trae ninguno y la fila de empaques es lo que más ancho pide.
let empaque: { id: number; name: string; qty: string; unitId: number } | null = null;

const h = () => ({ Authorization: `Bearer ${jwt}` });
async function api(request: APIRequestContext, method: 'get' | 'post' | 'put', path: string, data?: unknown) {
  const r = await request[method](`${API}${path}`, { headers: h(), data });
  if (!r.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${r.status()} ${await r.text()}`);
  const t = await r.text();
  return t ? JSON.parse(t) : null;
}

async function registrar(request: APIRequestContext, reasonId: number, qty = '0.001') {
  const res = await api(request, 'post', '/waste', { reasonId, ingredientId: insumo!.id, qty });
  registrados.push(res.id);
  return res as { id: number; status: string };
}

test.beforeAll(async ({ request }) => {
  jwt = await tokenDeRequest(request);
  topeOriginal = (await api(request, 'get', '/business-settings')).wasteApprovalThreshold ?? null;
  const motivos = await api(request, 'get', '/waste/reasons');
  for (let n = motivos.length + 1; n <= 8; n++) {
    const m = { name: `E2E motivo ${n}`, description: 'Motivo de prueba, se archiva al terminar' };
    const { id } = await api(request, 'post', '/waste/reasons', m);
    motivosCreados.push({ id, ...m });
  }
  let items = await api(request, 'get', '/waste/items');
  insumo = items.ingredients.find((i: { packs: unknown[] }) => i.packs.length > 0) ?? null;
  if (!insumo) {
    const base = items.ingredients[0];
    const unitId = base.units.find((u: { code: string }) => u.code === base.unitCode).id;
    const e = { name: 'E2E caja de 12', qty: '12', unitId };
    const creado = await api(request, 'post', `/stock/ingredients/${base.id}/presentations`, e);
    empaque = { id: creado.id, ...e };
    items = await api(request, 'get', '/waste/items');
    insumo = items.ingredients.find((i: { id: number }) => i.id === base.id);
  }
  console.log(`[e2e] insumo: ${insumo!.name} · empaques: ${insumo!.packs.length} · motivos creados: ${motivosCreados.length}`);
  // Un registro previo para que el insumo salga en «Recientes».
  if (!items.recent.some((r: { id: number }) => r.id === insumo!.id)) await registrar(request, motivos[0].id);
});

test.afterAll(async ({ request }) => {
  jwt = await tokenDeRequest(request);
  const pend: { id: number }[] = await api(request, 'get', '/waste/pending').catch(() => []);
  for (const id of registrados) {
    if (pend.some((p) => p.id === id)) await api(request, 'post', `/waste/${id}/reject`).catch((e) => console.log(`[e2e] rechazar ${id}: ${e}`));
    else await api(request, 'post', `/waste/${id}/undo`).catch((e) => console.log(`[e2e] deshacer ${id}: ${e}`));
  }
  await api(request, 'put', '/settings/waste-threshold', { threshold: topeOriginal });
  for (const m of motivosCreados) {
    await api(request, 'put', `/waste/reasons/${m.id}`, { name: m.name, description: m.description, isStaffMeal: false, archived: true });
  }
  if (empaque) {
    await api(request, 'put', `/stock/presentations/${empaque.id}`, { name: empaque.name, qty: empaque.qty, unitId: empaque.unitId, approximate: false, active: false });
  }
  console.log(`[e2e] empaque temporal desactivado: ${empaque?.id ?? 'no se creó'}`);
  console.log(`[e2e] limpieza: ${registrados.length} registros deshechos/rechazados (${registrados.join(',')}), tope → ${topeOriginal}, ${motivosCreados.length} motivos archivados`);
});

async function chicos(page: Page, raiz = 'body') {
  return page.locator(`${raiz} button:visible, ${raiz} input:visible, ${raiz} [role="tab"]:visible`).evaluateAll((els) =>
    els.map((e) => { const r = e.getBoundingClientRect(); return { t: (e.textContent || e.getAttribute('aria-label') || '').trim().slice(0, 30), h: Math.round(r.height), y: Math.round(r.y) }; })
      .filter((x) => x.h > 0 && x.h < 44 && x.y < 600 && !/^v[0-9a-f]{7}$/.test(x.t)));
// El folio de versión del riel (14 px) es anterior a esta feature y no es control de operación.
}

test('una pantalla cabe en 1024×600 con 8+ motivos y el insumo elegido', async ({ page }) => {
  await entrar(page);
  await page.goto('/merma');
  await expect(page.getByText('¿Por qué?')).toBeVisible();
  await page.getByRole('button', { name: 'Caducó' }).click();
  await page.getByRole('button', { name: insumo!.name, exact: true }).click();
  expect(insumo!.packs.length, 'el insumo no trae empaques: la fila no se mide').toBeGreaterThan(0);
  await page.getByRole('button', { name: insumo!.packs[0].name }).click();
  await expect(page.getByRole('button', { name: insumo!.packs[0].name })).toHaveAttribute('aria-pressed', 'true');
  await page.getByRole('button', { name: '1', exact: true }).click();
  await page.screenshot({ path: `${CAPTURAS}/una-pantalla.png` });
  const ancho = await page.evaluate(() => document.documentElement.scrollWidth);
  const altoDoc = await page.evaluate(() => document.documentElement.scrollHeight);
  const guardar = (await page.getByRole('button', { name: 'Guardar' }).boundingBox())!;
  const scrollY = await page.evaluate(() => window.scrollY);
  console.log(`[e2e] /merma scrollWidth=${ancho}px scrollHeight=${altoDoc}px Guardar y=${guardar.y} bottom=${guardar.y + guardar.height} h=${guardar.height} scrollY=${scrollY}`);
  const malos = await chicos(page);
  console.log(`[e2e] /merma controles <44px: ${JSON.stringify(malos)}`);
  expect(ancho).toBeLessThanOrEqual(1024);
  expect(scrollY).toBe(0);
  expect(guardar.y + guardar.height).toBeLessThanOrEqual(600);
  expect(malos).toEqual([]);
});

test('SC-001: motivo + reciente + Guardar son 3 toques', async ({ page }) => {
  await entrar(page);
  await page.goto('/merma');
  let toques = 0;
  await page.getByRole('button', { name: 'Caducó' }).click(); toques++;
  await page.getByRole('button', { name: insumo!.name, exact: true }).click(); toques++;
  await page.getByRole('button', { name: '1', exact: true }).click(); // teclear cantidad, no cuenta
  const resp = page.waitForResponse((r) => r.url().endsWith('/waste') && r.request().method() === 'POST');
  await page.getByRole('button', { name: 'Guardar' }).click(); toques++;
  const r = await resp;
  registrados.push((await r.json()).id);
  await expect(page.getByRole('status')).toContainText('Guardado');
  console.log(`[e2e] SC-001 toques=${toques}`);
  expect(toques).toBeLessThanOrEqual(3);
});

test('la franja sale en administración y no en /pos, /pedidos ni /merma', async ({ page, request }) => {
  await api(request, 'put', '/settings/waste-threshold', { threshold: '0.01' });
  const motivos = await api(request, 'get', '/waste/reasons');
  const res = await registrar(request, motivos[0].id, '1');
  expect(res.status, 'la merma no quedó pendiente con tope 0.01').toBe('pending');
  await entrar(page);
  await page.evaluate(() => sessionStorage.removeItem('egb:merma:franja-oculta'));
  const franja = page.getByRole('button', { name: /esperan? tu aprobación/ });
  await page.goto('/almacen');
  await expect(franja).toBeVisible({ timeout: 30_000 });
  const caja = (await franja.locator('..').boundingBox())!;
  console.log(`[e2e] franja en /almacen: alto=${caja.height}px y=${caja.y}`);
  await page.screenshot({ path: `${CAPTURAS}/franja-almacen.png` });
  expect(caja.height).toBeGreaterThanOrEqual(44);
  for (const ruta of ['/pos', '/pedidos', '/merma']) {
    await page.goto(ruta);
    await page.waitForLoadState('networkidle');
    const visible = await franja.isVisible();
    console.log(`[e2e] franja en ${ruta}: ${visible ? 'VISIBLE' : 'no'}`);
    expect(visible, `la franja sale en ${ruta}`).toBe(false);
  }
  await api(request, 'post', `/waste/${res.id}/reject`);
  registrados.splice(registrados.indexOf(res.id), 1);
  await api(request, 'put', '/settings/waste-threshold', { threshold: topeOriginal });
});

test('pestaña de Almacén: pestañas desplazables y fichas en una fila', async ({ page }) => {
  await entrar(page);
  await page.goto('/almacen?tab=merma');
  await expect(page.getByRole('button', { name: 'Esta semana' })).toBeVisible({ timeout: 30_000 });
  await page.waitForLoadState('networkidle');
  await page.screenshot({ path: `${CAPTURAS}/almacen-pestana.png` });
  const ancho = await page.evaluate(() => document.documentElement.scrollWidth);
  const lista = await page.getByRole('tablist').evaluate((e) => ({ sw: e.scrollWidth, cw: e.clientWidth, ox: getComputedStyle(e).overflowX }));
  const fichas = await page.locator('button[aria-pressed]').filter({ hasText: '$' }).evaluateAll((els) =>
    els.map((e) => { const r = e.getBoundingClientRect(); return { y: Math.round(r.y), h: Math.round(r.height) }; }));
  const filas = new Set(fichas.map((f) => f.y)).size;
  const malos = await chicos(page);
  console.log(`[e2e] almacén scrollWidth=${ancho}px tablist sw=${lista.sw} cw=${lista.cw} overflowX=${lista.ox}; fichas=${fichas.length} filas=${filas} alto=${fichas[0]?.h}; <44: ${JSON.stringify(malos)}`);
  expect(ancho).toBeLessThanOrEqual(1024);
  expect(['auto', 'scroll']).toContain(lista.ox);
  expect(filas).toBeLessThanOrEqual(1);
});
