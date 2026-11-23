import { test, expect, type Page } from '@playwright/test';
import { API, tokenDeApi } from './ambiente';
import { iniciarSesion } from './sesion';

// MERMA ESPERADA (spec 096, subida 1) contra la API real del worktree, a 1024×600 (pedido del dueño)
// y a 1280×854. Captura la merma general, la de un producto y la de un artículo, revisa que el
// desglose cuadre y deja todo como estaba. Las capturas van a test-results/, nunca al repositorio.
const PRODUCTO = process.env.E2E_MERMA_PRODUCTO ?? 'Capuccino';
const ARTICULO = process.env.E2E_MERMA_ARTICULO ?? 'Palillos chinos - 1 par';

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const jwt = await tokenDeApi();
  const r = await fetch(`${API}${path}`, { ...init, headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json', ...(init?.headers ?? {}) } });
  if (!r.ok) throw new Error(`${init?.method ?? 'GET'} ${path}: ${r.status}`);
  return r.status === 204 ? (undefined as T) : r.json();
}
const put = (path: string, pct: number | null) => api(path, { method: 'PUT', body: JSON.stringify({ pct }) });
const num = (v: string | null | undefined) => (v == null ? null : Number(v));

let antes: { general: number | null; producto: number | null; articulo: number | null; productoId: number; articuloId: number };

test.beforeAll(async () => {
  const bs = await api<{ expectedWastePct?: string | null }>('/business-settings');
  const prods = await api<{ items: { id: number; name: string; expectedWastePct?: string | null }[] }>(`/admin/products?search=${encodeURIComponent(PRODUCTO)}&limit=10`);
  const p = prods.items.find((x) => x.name === PRODUCTO);
  const arts = await api<{ id: number; name: string }[]>('/stock/articles');
  const a = arts.find((x) => x.name === ARTICULO);
  if (!p || !a) throw new Error(`falta el producto «${PRODUCTO}» o el artículo «${ARTICULO}» en la base: fíjalos con E2E_MERMA_*`);
  const ficha = await api<{ expectedWastePct?: string | null }>(`/stock/articles/${a.id}`);
  antes = { general: num(bs.expectedWastePct), producto: num(p.expectedWastePct), articulo: num(ficha.expectedWastePct), productoId: p.id, articuloId: a.id };
});

test.afterAll(async () => {
  if (!antes) return;
  await put('/settings/expected-waste', antes.general);
  await put(`/admin/products/${antes.productoId}/expected-waste`, antes.producto);
  await put(`/stock/articles/${antes.articuloId}/expected-waste`, antes.articulo);
});

async function entrar(page: Page, path: string) {
  await page.goto(path);
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  await page.goto(path);
}

for (const [w, h] of [[1024, 600], [1280, 854]] as const) {
  test.describe(`a ${w}×${h}`, () => {
    test.use({ viewport: { width: w, height: h }, hasTouch: true });

    test('la merma general se captura en Ajustes › Costos en ≤ 4 toques y mueve el costo', async ({ page }) => {
      // Desde cero: con un % propio del producto (lo deja el caso del producto), el general no lo mueve.
      await put('/settings/expected-waste', null);
      await put(`/admin/products/${antes.productoId}/expected-waste`, null);
      const sin = await api<{ cost: string }>(`/products/${antes.productoId}/costing`);
      await entrar(page, '/negocio?seccion=costos');
      let toques = 0;
      const campo = page.getByLabel('Merma al preparar, en porcentaje');
      await campo.tap(); toques++;
      await campo.fill('5');
      await page.getByRole('button', { name: 'Guardar' }).tap(); toques++;
      await expect.poll(async () => num((await api<{ expectedWastePct?: string | null }>('/business-settings')).expectedWastePct)).toBe(5);
      expect(toques, 'toques para capturar la merma general (SC-005)').toBeLessThanOrEqual(4);
      const con = await api<{ cost: string; base: string; waste: string; lines: { waste: string }[] }>(`/products/${antes.productoId}/costing`);
      expect(Number(con.cost)).toBeGreaterThan(Number(sin.cost));
      const suma = con.lines.reduce((s, l) => s + Number(l.waste), 0);
      expect(Math.abs(suma - Number(con.waste)), 'la suma de renglones es el total de merma (SC-003)').toBeLessThan(0.00011);
      await page.screenshot({ path: `test-results/merma-ajustes-${w}.png` });
    });

    test('el producto: pestaña Costo, su % en ≤ 4 toques y el desglose', async ({ page }) => {
      await entrar(page, '/catalogo/productos');
      await page.getByPlaceholder(/Buscar/).first().fill(PRODUCTO);
      const fila = page.getByRole('row').filter({ hasText: PRODUCTO }).first();
      await expect(fila).toBeVisible({ timeout: 15_000 });
      let toques = 0;
      await fila.getByRole('button', { name: 'Editar' }).tap(); toques++;
      const dialogo = page.getByRole('dialog');
      await dialogo.getByRole('tab', { name: 'Costo' }).tap(); toques++;
      const campo = dialogo.getByLabel('Merma del producto, en porcentaje');
      await campo.fill('10');
      await expect(dialogo.getByText(/de este producto/).first()).toBeVisible();
      await page.screenshot({ path: `test-results/merma-producto-${w}.png` });
      await dialogo.getByRole('button', { name: 'Guardar' }).tap(); toques++;
      await expect.poll(async () => {
        const c = await api<{ productPct: number | null }>(`/products/${antes.productoId}/costing`);
        return c.productPct;
      }).toBe(10);
      expect(toques, 'toques para capturar la merma del producto (SC-005), sin contar teclear').toBeLessThanOrEqual(4);
    });

    test('el artículo: 0 % capturado es distinto de sin capturar, y el historial sigue a la vista', async ({ page }) => {
      await put(`/stock/articles/${antes.articuloId}/expected-waste`, null);
      await entrar(page, '/almacen?tab=insumos');
      const renglon = page.getByRole('button', { name: ARTICULO, exact: true }).first();
      await expect(renglon).toBeVisible({ timeout: 30_000 });
      await renglon.click();
      await expect(page.getByText('Merma al preparar')).toBeVisible();
      // La ficha puede ofrecer configurar alertas la primera vez (spec 084); no es parte de esta prueba.
      const noAhora = page.getByRole('button', { name: 'No, por ahora' });
      if (await noAhora.isVisible({ timeout: 2_000 }).catch(() => false)) await noAhora.tap();
      await page.getByRole('button', { name: 'Capturada' }).tap();
      const campo = page.getByLabel('Merma del artículo, en porcentaje');
      await campo.fill('0');
      await campo.blur();
      await expect(page.getByText('Aquí no se pierde nada')).toBeVisible();
      // Capturado en 0, no nulo: «aquí no se pierde nada» es distinto de «sin capturar».
      await expect.poll(async () => num((await api<{ expectedWastePct?: string | null }>(`/stock/articles/${antes.articuloId}`)).expectedWastePct)).toBe(0);
      await expect(page.getByText('Merma real, últimos 90 días')).toBeVisible();
      // El historial de compras no se queda sin lugar: su encabezado sigue dentro de la pantalla.
      const hist = await page.getByText('Compras y correcciones, la más reciente arriba').boundingBox();
      expect(hist && hist.y + hist.height, 'el historial quedó fuera de la pantalla').toBeLessThan(h - 96);
      await page.screenshot({ path: `test-results/merma-articulo-${w}.png` });
    });
  });
}
