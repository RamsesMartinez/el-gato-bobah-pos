import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
import { entrar } from './pos';
import { API, tokenDeRequest } from './ambiente';

// UNIR INSUMOS (spec 056): lo que jsdom no mide. A 1024×600 y con toque: la hoja de unir y la de
// posibles duplicados no se salen de lado, todo lo tocable mide 44 px, y unir de verdad deja la
// existencia del destino como la suma. Crea sus propios insumos (con nombre de esta corrida) y al
// terminar archiva el que quedó; no toca ninguno de la base.

test.use({ hasTouch: true });

const CAPTURAS = process.env.E2E_CAPTURAS ?? 'test-results/unir-insumos';
const corrida = Date.now().toString(36);

async function crear(request: APIRequestContext, token: string, name: string, stock: string): Promise<number> {
  const units = await (await request.get(`${API}/units`, { headers: { Authorization: `Bearer ${token}` } })).json();
  const ml = units.items.find((u: { code: string }) => u.code === 'ml').id;
  const r = await request.post(`${API}/stock/articles`, {
    headers: { Authorization: `Bearer ${token}` }, data: { name, baseUnitId: ml, initialStock: stock, initialCost: '0.02' },
  });
  expect(r.ok(), `crear ${name}: ${r.status()}`).toBeTruthy();
  return (await r.json()).id;
}

async function chicos(page: Page) {
  return page.evaluate(() => [...document.querySelectorAll('[role="dialog"] button,[role="dialog"] input,[role="dialog"] [role=checkbox],[role="dialog"] [role=radio]')]
    .filter((e) => { const r = e.getBoundingClientRect(); return r.width > 0 && r.height > 0 && r.bottom > 0 && r.top < innerHeight; })
    .map((e) => ({ alto: Math.round(e.getBoundingClientRect().height), texto: ((e as HTMLElement).innerText || e.getAttribute('aria-label') || '').trim().slice(0, 40) }))
    .filter((c) => c.alto < 44));
}

const deLado = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);

test('unir dos marcas desde la ficha: vista previa, confirmar y la existencia queda sumada', async ({ page, request }) => {
  const token = await tokenDeRequest(request);
  const destino = `Leche e2e ${corrida}`;
  const origen = `Leche e2e ${corrida} marca B`;
  const a = await crear(request, token, destino, '500');
  await crear(request, token, origen, '300');

  await entrar(page);
  await page.goto(`/almacen?tab=insumos&open=${a}`);
  await expect(page.getByRole('button', { name: /Volver a Artículos/ })).toBeVisible({ timeout: 15_000 });
  await page.getByRole('button', { name: 'Unir otros aquí' }).click();
  const hoja = page.locator('[role="dialog"]').last();
  await expect(hoja.getByText(`Unir en ${destino}`)).toBeVisible();
  await hoja.getByRole('checkbox', { name: new RegExp(origen) }).click();
  await page.screenshot({ path: `${CAPTURAS}/elegir.png` });
  expect(await chicos(page)).toEqual([]);
  expect(await deLado(page)).toBeLessThanOrEqual(0);

  await hoja.getByRole('button', { name: 'Siguiente ›' }).click();
  await expect(hoja.getByText('Antes de unir')).toBeVisible();
  await page.screenshot({ path: `${CAPTURAS}/vista-previa.png` });
  expect(await chicos(page)).toEqual([]);
  expect(await deLado(page)).toBeLessThanOrEqual(0);
  await expect(hoja.getByRole('button', { name: 'Unir' })).toBeInViewport();
  await hoja.getByRole('button', { name: 'Unir' }).click();
  await expect(page.getByText(/quedó archivado/)).toBeVisible();

  const ficha = await (await request.get(`${API}/stock/articles/${a}`, { headers: { Authorization: `Bearer ${token}` } })).json();
  expect(Number(ficha.onHand), 'el destino tiene la suma').toBe(800);
  expect(ficha.mergedSources).toBe(1);
  // Limpieza: el destino se archiva (el origen ya quedó archivado por la unión).
  await request.post(`${API}/stock/articles/${a}/archive`, { headers: { Authorization: `Bearer ${token}` } });
});

test('al dar de alta un nombre parecido, pregunta antes de crear y deja abrir el existente', async ({ page, request }) => {
  const token = await tokenDeRequest(request);
  const existente = `Polvo e2e ${corrida}`;
  const id = await crear(request, token, existente, '0');
  await entrar(page);
  await page.goto('/almacen?tab=insumos');
  await page.getByRole('button', { name: '+ Nuevo' }).click();
  const hoja = page.locator('[role="dialog"]').last();
  await hoja.getByLabel(/Nombre/).fill(`${existente} chocolate`);
  await hoja.getByRole('button', { name: 'Líquido' }).click();
  await hoja.getByRole('button', { name: 'Guardar' }).click();
  await expect(hoja.getByText('¿Es alguno de estos?')).toBeVisible();
  await page.screenshot({ path: `${CAPTURAS}/aviso.png` });
  expect(await chicos(page)).toEqual([]);
  await hoja.getByRole('button', { name: `Abrir ${existente}` }).click();
  await expect(page.getByRole('button', { name: /Volver a Artículos/ })).toBeVisible();
  await request.post(`${API}/stock/articles/${id}/archive`, { headers: { Authorization: `Bearer ${token}` } });
});

test('posibles duplicados: el botón cabe en la barra y el grupo abierto mide 44 px (solo lee)', async ({ page }) => {
  await entrar(page);
  await page.goto('/almacen?tab=insumos');
  const boton = page.getByRole('button', { name: /Posibles duplicados/ });
  test.skip(!(await boton.waitFor({ timeout: 10_000 }).then(() => true, () => false)), 'la base no tiene posibles duplicados');
  expect(await deLado(page), 'la barra de Artículos se desplaza de lado').toBeLessThanOrEqual(0);
  await boton.click();
  const hoja = page.locator('[role="dialog"]').last();
  await hoja.getByRole('button', { name: /Revisar ›/ }).first().click();
  await expect(hoja.getByText('¿Cuál se queda?')).toBeVisible();
  await page.screenshot({ path: `${CAPTURAS}/duplicados.png` });
  expect(await chicos(page)).toEqual([]);
  expect(await deLado(page)).toBeLessThanOrEqual(0);
  await hoja.getByRole('button', { name: 'Listo' }).click();
});
