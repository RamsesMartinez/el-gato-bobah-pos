import { test, expect, type APIRequestContext } from '@playwright/test';
import { API, tokenDeRequest } from './ambiente';
import { entrar } from './pos';

// LUGARES PARA CONTAR (spec 040, T018): lo que jsdom no puede medir, porque ahí todo mide 0 px.
// Que desplazar la lista con el dedo no mueva un lugar, que mantener 500 ms sobre la agarradera sí,
// que arrastrar de la fila 10 a la 1 desplace solo, y que el lugar del artículo se elija en dos
// toques sin abrir el teclado. El ambiente lo usa una persona: los lugares que crea esta prueba se
// archivan y el artículo vuelve al lugar que tenía.

test.describe.configure({ mode: 'serial', retries: 0 });
test.use({ hasTouch: true, viewport: { width: 1024, height: 600 } });

const CAPTURAS = process.env.E2E_CAPTURAS ?? 'test-results/lugares-para-contar';
let jwt = '';
const creados: number[] = [];
let articulo: { id: number; name: string; placeId: number | null } | null = null;

async function api(request: APIRequestContext, method: 'get' | 'post' | 'put', path: string, data?: unknown) {
  const r = await request[method](`${API}${path}`, { headers: { Authorization: `Bearer ${jwt}` }, data });
  if (!r.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${r.status()} ${await r.text()}`);
  const t = await r.text();
  return t ? JSON.parse(t) : null;
}
const nombres = async (request: APIRequestContext) => (await api(request, 'get', '/stock/count-places')).map((p: { name: string }) => p.name);

test.beforeAll(async ({ request }) => {
  jwt = await tokenDeRequest(request);
  const ya: string[] = await nombres(request);
  for (let n = 1; ya.length + creados.length < 10; n++) {
    const name = `E2E lugar ${n}`;
    if (ya.includes(name)) continue;
    creados.push((await api(request, 'post', '/stock/count-places', { name })).id);
  }
  const ings = await api(request, 'get', '/ingredients');
  const i = (Array.isArray(ings) ? ings : ings.items)[0];
  articulo = { id: i.id, name: i.name, placeId: i.countPlaceId ?? null };
});

test.afterAll(async ({ request }) => {
  jwt = await tokenDeRequest(request);
  if (articulo) await api(request, 'put', `/stock/ingredients/${articulo.id}/place`, { placeId: articulo.placeId }).catch(() => {});
  for (const id of creados) await api(request, 'post', `/stock/count-places/${id}/archive`, { moveTo: null }).catch(() => {});
  console.log(`[e2e] limpieza: ${creados.length} lugares archivados; el artículo volvió a ${articulo?.placeId ?? 'sin lugar'}`);
});

test('desplazar la lista no mueve nada; 500 ms sobre ≡ levanta y arrastrar de la 10 a la 1 desplaza solo', async ({ page, request }) => {
  await entrar(page);
  await page.goto('/negocio');
  const titulo = page.getByText('Lugares', { exact: true });
  await titulo.scrollIntoViewIfNeeded();
  const antes: string[] = await nombres(request);

  // Un deslizamiento sobre el texto del renglón (no la agarradera) desplaza y no mueve.
  const texto = page.getByText(antes[2], { exact: true });
  const caja = (await texto.boundingBox())!;
  await page.mouse.move(caja.x + 20, caja.y + caja.height / 2);
  await page.mouse.down();
  await page.mouse.move(caja.x + 20, caja.y - 150, { steps: 10 });
  await page.mouse.up();
  await page.waitForTimeout(800);
  expect(await nombres(request), 'deslizar sobre el renglón movió un lugar').toEqual(antes);

  // Del último al primero: mantener, arrastrar hacia arriba y esperar el desplazamiento automático.
  const ultimo = antes[antes.length - 1];
  const grip = page.getByRole('button', { name: `Mover ${ultimo}` });
  await grip.scrollIntoViewIfNeeded();
  const g = (await grip.boundingBox())!;
  await page.mouse.move(g.x + g.width / 2, g.y + g.height / 2);
  await page.mouse.down();
  await page.waitForTimeout(650);
  await page.mouse.move(g.x + g.width / 2, g.y - 20, { steps: 5 });
  await page.screenshot({ path: `${CAPTURAS}/levantado.png` });
  // Como una persona: acerca el dedo a la orilla, deja que la lista suba un poco, lo regresa al
  // centro para frenar y mira si ya ve el primero. Sostenerlo en la orilla sube sin parar.
  const primero = page.getByRole('button', { name: `Mover ${antes[0]}` });
  let p = await primero.boundingBox();
  for (let i = 0; i < 30 && !(p && p.y > 40 && p.y < 520); i++) {
    await page.mouse.move(g.x + g.width / 2, 10, { steps: 4 });
    await page.waitForTimeout(250);
    await page.mouse.move(g.x + g.width / 2, 300, { steps: 4 });
    await page.waitForTimeout(150);
    p = await primero.boundingBox();
  }
  expect(p && p.y > 40 && p.y < 520, 'la lista no subió sola hasta el primer lugar').toBeTruthy();
  await page.mouse.move(p!.x + p!.width / 2, p!.y - 30, { steps: 10 });
  await page.waitForTimeout(300);
  await page.screenshot({ path: `${CAPTURAS}/antes-de-soltar.png` });
  await page.mouse.up();
  await expect.poll(async () => (await nombres(request))[0], { timeout: 10_000 }).toBe(ultimo);
  // El aviso para lectores de pantalla, en español y con el nombre (no «Draggable item 95…»).
  const avisos = (await page.locator('[aria-live]').allTextContents()).join(' ');
  expect(avisos, 'aviso del arrastre').toContain(`«${ultimo}» quedó en el lugar de`);
  expect(avisos).not.toMatch(/Draggable item/);
  await page.screenshot({ path: `${CAPTURAS}/arrastrado.png` });
  // Regresa el orden que tenía.
  const ids = (await api(request, 'get', '/stock/count-places')) as { id: number; name: string }[];
  for (const [pos, name] of antes.entries()) {
    const id = ids.find((x) => x.name === name)!.id;
    await api(request, 'post', `/stock/count-places/${id}/move`, { toPosition: pos });
  }
});

test('el lugar del artículo se elige en dos toques y sin teclado', async ({ page, request }) => {
  await entrar(page);
  await page.goto('/almacen');
  await page.getByRole('tab', { name: 'Artículos' }).click();
  // T025/T027: el lugar tiene su propio botón en el renglón del artículo, aparte de los empaques.
  const fila = page.getByRole('row').filter({ has: page.getByRole('button', { name: `Cómo llega ${articulo!.name}` }) });
  await expect(page.getByRole('button', { name: `Cómo llega ${articulo!.name}` })).toContainText('Empaques');
  const renglon = fila.getByText('Lugar', { exact: true }).locator('..').getByRole('button').first();
  await expect(renglon).toBeVisible();
  const destino: string = (await nombres(request))[1];
  let toques = 0;
  await renglon.click(); toques++;
  await expect(page.getByRole('button', { name: destino, exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.activeElement?.tagName), 'el teclado del sistema se abrió solo').not.toBe('INPUT');
  await page.screenshot({ path: `${CAPTURAS}/hoja-del-lugar.png` });
  await page.getByRole('button', { name: destino, exact: true }).click(); toques++;
  await expect(renglon).toContainText(destino);
  console.log(`[e2e] toques para elegir lugar=${toques}`);
  expect(toques).toBe(2);
});

// T026: desde el conteo, sin conteo abierto, se ve el orden del recorrido y se llega a cambiarlo.
test('el conteo dice el orden de los lugares y lleva a ordenarlos', async ({ page }) => {
  await entrar(page);
  await page.goto('/almacen');
  await page.getByRole('tab', { name: 'Conteo' }).click();
  await expect(page.getByText(/^Se recorre en este orden: /)).toBeVisible();
  await page.getByRole('button', { name: 'Cambiar el orden ›' }).click();
  await expect(page).toHaveURL(/\/negocio#lugares$/);
  await expect(page.getByText('Mantén presionada ≡ para mover un lugar')).toBeInViewport();
  await page.getByRole('button', { name: '‹ Volver al conteo' }).click();
  await expect(page.getByText(/^Se recorre en este orden: /)).toBeVisible();
  await page.screenshot({ path: `${CAPTURAS}/desde-el-conteo.png` });
});
