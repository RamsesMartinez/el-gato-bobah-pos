import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
import { iniciarSesion } from './sesion';
import { API, tokenDeRequest } from './ambiente';

// DENSIDAD Y TARA (spec 058): lo que jsdom no mide. A 1024×600 y con toque: la hoja de densidad con el
// teclado abierto y la captura del conteo con la pesada desplegada no sacan «Guardar» de la pantalla,
// todo lo tocable mide 44 px, y lo que se ve es lo que el servidor guarda. Crea sus propios artículos
// (con nombre de esta corrida), descarta el conteo que abrió y archiva lo que creó.

test.use({ hasTouch: true });

const CAPTURAS = process.env.E2E_CAPTURAS ?? 'test-results/densidad-y-tara';
const corrida = Date.now().toString(36);

const auth = (token: string) => ({ Authorization: `Bearer ${token}` });

async function unidad(request: APIRequestContext, token: string, code: string): Promise<number> {
  const units = await (await request.get(`${API}/units`, { headers: auth(token) })).json();
  return units.items.find((u: { code: string }) => u.code === code).id;
}

async function crear(request: APIRequestContext, token: string, name: string): Promise<number> {
  const r = await request.post(`${API}/stock/articles`, {
    headers: auth(token), data: { name, baseUnitId: await unidad(request, token, 'ml'), initialStock: '0', initialCost: '0.1' },
  });
  expect(r.ok(), `crear ${name}: ${r.status()}`).toBeTruthy();
  return (await r.json()).id;
}

async function chicos(page: Page) {
  return page.evaluate(() => [...document.querySelectorAll('[role="dialog"] button,[role="dialog"] input')]
    .filter((e) => { const r = e.getBoundingClientRect(); return r.width > 0 && r.height > 0 && r.bottom > 0 && r.top < innerHeight; })
    .map((e) => ({ alto: Math.round(e.getBoundingClientRect().height), texto: ((e as HTMLElement).innerText || e.getAttribute('aria-label') || '').trim().slice(0, 40) }))
    .filter((c) => c.alto < 44));
}

const deLado = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);

async function teclear(page: Page, valor: string) {
  const hoja = page.locator('[role="dialog"]').last();
  for (const ch of valor) await hoja.getByRole('button', { name: ch === '.' ? 'punto' : ch, exact: true }).click();
}

async function entrar(page: Page, ruta: string) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  await page.goto(ruta);
}

test('capturar la densidad con una pesada: la cuenta se ve antes de guardar y la compra en kg entra en ml', async ({ page, request }) => {
  const token = await tokenDeRequest(request);
  const nombre = `Salsa e2e ${corrida}`;
  const id = await crear(request, token, nombre);

  await entrar(page, `/almacen?tab=insumos&open=${id}`);
  await expect(page.getByRole('button', { name: /Volver a Artículos/ })).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText('Sin capturar')).toBeVisible();
  await page.getByRole('button', { name: 'Capturar la densidad' }).click();
  const hoja = page.locator('[role="dialog"]').last();
  await expect(hoja.getByText(`Densidad · ${nombre}`)).toBeVisible();

  await hoja.getByLabel('Pesa lleno (g)').click();
  await teclear(page, '1250');
  await page.screenshot({ path: `${CAPTURAS}/pesada-con-teclado.png` });
  expect(await chicos(page)).toEqual([]);
  expect(await deLado(page)).toBeLessThanOrEqual(0);
  await hoja.getByLabel('Pesa la botella vacía (g)').click();
  await teclear(page, '150');
  await hoja.getByLabel(/Contiene \(ml/).click();
  await teclear(page, '1000');
  await expect(hoja.getByTestId('resultado')).toContainText('1.10');
  await hoja.getByRole('button', { name: 'Listo' }).click();
  await expect(hoja.getByRole('button', { name: 'Guardar' })).toBeInViewport();
  await hoja.getByRole('button', { name: 'Guardar' }).click();
  await expect(page.getByText(/1\.10 gramos por ml · por pesada/)).toBeVisible();

  // La compra la hace el servidor con la densidad que se acaba de guardar: 4.3 kg ÷ 1.10.
  const cats = await (await request.get(`${API}/expense-categories`, { headers: auth(token) })).json();
  const categoryId = (cats.items ?? cats)[0].id;
  const kg = await unidad(request, token, 'kg');
  const hoy = new Date().toISOString().slice(0, 10);
  const r = await request.post(`${API}/expenses`, {
    headers: auth(token), data: {
      expenseDate: hoy, receivedAt: hoy, categoryId, amount: '430', status: 'pendiente', description: `e2e ${corrida}`,
      items: [{ itemType: 'ingrediente', ingredientId: id, description: 'Galón', quantity: '4.3', unitId: kg, qtyReceived: '4.3', amount: '430' }],
    },
  });
  expect(r.ok(), `compra en kg: ${r.status()} ${await r.text()}`).toBeTruthy();
  const gasto = (await r.json()).id;
  const ficha = await (await request.get(`${API}/stock/articles/${id}`, { headers: auth(token) })).json();
  expect(Number(ficha.onHand)).toBeCloseTo(3909.0909, 3);

  await request.post(`${API}/expenses/${gasto}/cancel`, { headers: auth(token), data: { reason: 'e2e' } });
  await request.post(`${API}/stock/articles/${id}/archive`, { headers: auth(token) });
});

test('contar pesando con el empaque: «Guardar» se queda en pantalla y se guarda la cifra del servidor', async ({ page, request }) => {
  const token = await tokenDeRequest(request);
  const nombre = `Jarabe e2e ${corrida}`;
  const id = await crear(request, token, nombre);
  const d = await request.put(`${API}/stock/articles/${id}/density`, { headers: auth(token), data: { method: 'manual', density: '1.1' } });
  expect(d.ok()).toBeTruthy();
  const ml = await unidad(request, token, 'ml');
  const p = await request.post(`${API}/stock/ingredients/${id}/presentations`, {
    headers: auth(token), data: { name: 'Botella', qty: '1000', unitId: ml, approximate: false, tareG: '120' },
  });
  expect(p.ok(), `empaque: ${p.status()}`).toBeTruthy();

  const abierto = await request.get(`${API}/stock/counts/current`, { headers: auth(token) });
  test.skip(abierto.status() === 200, 'hay un conteo abierto de alguien: no se toca');
  const c = await request.post(`${API}/stock/counts`, { headers: auth(token) });
  expect(c.status()).toBe(201);
  const conteo = (await c.json()).id;
  try {
    await entrar(page, '/almacen?tab=conteo');
    await page.getByLabel('Buscar').fill(nombre);
    const renglon = page.getByTestId('renglon-conteo').filter({ hasText: nombre });
    await renglon.getByRole('button', { name: 'Contar' }).click();
    const hoja = page.locator('[role="dialog"]').last();
    await hoja.getByRole('button', { name: /Pesar con empaque/ }).click();
    await hoja.getByLabel(`Peso en la báscula de ${nombre}`).click();
    await teclear(page, '850');
    // Empieza en un empaque pesado: uno más son los dos de la báscula.
    await hoja.getByRole('button', { name: 'Agregar un empaque pesado' }).click();
    await expect(hoja.getByText(/850 g − 2 × 120 g = 610 g ÷ 1\.10/)).toBeVisible();
    await page.screenshot({ path: `${CAPTURAS}/pesar-con-empaque.png` });
    expect(await chicos(page)).toEqual([]);
    expect(await deLado(page)).toBeLessThanOrEqual(0);
    await expect(hoja.getByRole('button', { name: 'Guardar' })).toBeInViewport();
    await hoja.getByRole('button', { name: 'Guardar' }).click();
    await expect(renglon).toContainText(/554/);
  } finally {
    await request.post(`${API}/stock/counts/${conteo}/discard`, { headers: auth(token), data: {} });
    await request.post(`${API}/stock/articles/${id}/archive`, { headers: auth(token) });
  }
});
