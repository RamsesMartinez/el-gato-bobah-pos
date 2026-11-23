import { randomUUID } from 'node:crypto';
import { test, expect, type Page } from '@playwright/test';
import { API, tokenDeApi } from './ambiente';
import { iniciarSesion } from './sesion';

// TRASPASO DE BODEGA A LOCAL (spec 094, T054b) contra la API real del worktree, a 1280×854 y a
// 1024×600. Desde Existencias se lleva una pieza de un artículo de una bodega al almacén de venta en
// 6 toques o menos (SC-003), sin contar lo que se teclea, y la API dice que el origen bajó y el
// destino subió exactamente eso, con un movimiento de traspaso de cada lado que nombra al otro.
//
// Lo que la prueba necesita lo crea y lo deja como estaba:
// - Un almacén «Bodega e2e …» propio, para que su lista tenga un solo renglón y ningún dato de la
//   base decida qué se ve. Se archiva al final.
// - La existencia de esa bodega sale del almacén de venta con un traspaso por API (no con un conteo
//   ni una compra): así lo que la bodega tiene ya estaba en el negocio, y el traspaso de regreso del
//   final deja el almacén de venta con lo mismo que tenía antes de empezar. Un conteo o una compra
//   inventarían mercancía que no existe y la regresarían al almacén de venta.
// - El artículo es el primero en piezas con al menos 5 en el almacén de venta: ningún id ni nombre
//   fijo. Las capturas van a test-results/, nunca al repositorio.
const LLEVAR_A_LA_BODEGA = 3; // piezas; cada prueba regresa 1 desde la pantalla

type Nivel = { item_type: string; ingredient_id?: number | null; product_id?: number | null; item_name: string; on_hand: string; unit_code: string; is_active?: boolean };
type Almacen = { id: number; name: string; isSales: boolean; isActive: boolean };
type Movimiento = { id: number; item_type: string; item_name: string; movement_type: string; quantity: string; transfer_id?: number | null; counterpart_warehouse_name?: string | null; warehouse_id?: number };

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const jwt = await tokenDeApi();
  const r = await fetch(`${API}${path}`, { ...init, headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json', ...(init?.headers ?? {}) } });
  if (!r.ok) throw new Error(`${init?.method ?? 'GET'} ${path}: ${r.status} ${await r.text()}`);
  return r.status === 204 ? (undefined as T) : r.json();
}
const niveles = (almacen: number) => api<{ items: Nivel[] }>(`/stock/levels?warehouseId=${almacen}`).then((r) => r.items ?? []);
const movimientos = (almacen: number) => api<{ items: Movimiento[] }>(`/stock/movements?warehouseId=${almacen}`).then((r) => r.items ?? []);
const idDe = (n: Nivel) => n.ingredient_id ?? n.product_id ?? null;

let ctx: { venta: Almacen; bodega: Almacen; articulo: Nivel; localAntes: number } | undefined;

async function existencia(almacen: number): Promise<number> {
  const n = (await niveles(almacen)).find((x) => x.item_type === ctx!.articulo.item_type && idDe(x) === idDe(ctx!.articulo));
  return Number(n?.on_hand ?? 0);
}

async function traspasarPorApi(de: number, a: number, qty: number) {
  const a0 = ctx!.articulo;
  await api('/stock/transfers', {
    method: 'POST',
    body: JSON.stringify({
      clientKey: randomUUID(), fromWarehouseId: de, toWarehouseId: a, note: 'e2e traspaso',
      lines: [{ itemType: a0.item_type, ...(a0.item_type === 'producto' ? { productId: idDe(a0) } : { ingredientId: idDe(a0) }), qty: String(qty) }],
    }),
  });
}

test.beforeAll(async () => {
  const almacenes = (await api<{ items?: Almacen[] }>('/stock/warehouses')).items ?? [];
  const venta = almacenes.find((w) => w.isSales);
  if (!venta) throw new Error('la sucursal no tiene almacén de venta');
  const articulo = (await niveles(venta.id)).find((n) => n.unit_code === 'pieza' && n.is_active !== false && idDe(n) !== null && Number(n.on_hand) >= 5);
  if (!articulo) throw new Error(`${venta.name} no tiene un artículo en piezas con al menos 5: la prueba no tiene qué llevar`);
  const nombre = `Bodega e2e ${Date.now().toString(36)}`;
  const { id } = await api<{ id: number }>('/stock/warehouses', { method: 'POST', body: JSON.stringify({ name: nombre }) });
  ctx = { venta, bodega: { id, name: nombre, isSales: false, isActive: true }, articulo, localAntes: Number(articulo.on_hand) };
  await traspasarPorApi(venta.id, id, LLEVAR_A_LA_BODEGA);
  expect(await existencia(id), 'la bodega de la prueba no recibió lo del setup').toBe(LLEVAR_A_LA_BODEGA);
});

test.afterAll(async () => {
  if (!ctx) return;
  const resto = await existencia(ctx.bodega.id);
  if (resto > 0) await traspasarPorApi(ctx.bodega.id, ctx.venta.id, resto);
  await api(`/stock/warehouses/${ctx.bodega.id}/archive`, { method: 'POST', body: '{}' });
  expect(await existencia(ctx.venta.id), `${ctx.venta.name} no quedó con lo que tenía antes de la prueba`).toBe(ctx.localAntes);
});

async function entrar(page: Page, path: string) {
  await page.goto(path);
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  await page.goto(path);
}

for (const [w, h] of [[1280, 854], [1024, 600]] as const) {
  test.describe(`a ${w}×${h}`, () => {
    test.use({ viewport: { width: w, height: h }, hasTouch: true });

    test('una pieza de la bodega al almacén de venta, en ≤ 6 toques desde Existencias', async ({ page }) => {
      const { venta, bodega, articulo } = ctx!;
      const bodegaAntes = await existencia(bodega.id);
      const ventaAntes = await existencia(venta.id);
      expect(bodegaAntes, 'la bodega de la prueba se quedó sin qué llevar').toBeGreaterThanOrEqual(1);

      await entrar(page, '/almacen');
      await expect(page.getByRole('tab', { name: 'Existencias' })).toHaveAttribute('aria-selected', 'true', { timeout: 30_000 });

      let toques = 0;
      await page.getByRole('tab', { name: 'Traspaso' }).tap(); toques++;
      // El origen por omisión es el primer almacén que no es de venta, y la base puede tener otros:
      // se elige la bodega de la prueba como lo haría quien opera.
      await page.getByRole('group', { name: 'De' }).getByRole('button').tap(); toques++;
      await page.getByRole('dialog').getByRole('button', { name: bodega.name, exact: true }).tap(); toques++;
      await expect(page.getByRole('group', { name: 'A' }).getByRole('button')).toContainText(venta.name);

      const renglon = page.getByRole('row').filter({ hasText: articulo.item_name });
      await expect(renglon).toBeVisible();
      // La bodega es nueva: el artículo de la prueba es lo único que hay que llevar.
      await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: 'Más' }) })).toHaveCount(1);
      await renglon.getByRole('button', { name: 'Más' }).tap(); toques++;
      await expect(renglon.getByLabel(`Cuánto de ${articulo.item_name}`)).toHaveValue('1');

      const traspasar = page.getByRole('button', { name: 'Traspasar 1 artículo' });
      await expect(traspasar).toBeInViewport();
      await traspasar.tap(); toques++;
      await expect(page.getByText(`Listo: 1 pz de ${articulo.item_name} de ${bodega.name} a ${venta.name}`)).toBeVisible();
      await page.screenshot({ path: `test-results/traspaso-${w}x${h}.png` });

      console.log(`[e2e] traspaso a ${w}×${h}: ${toques} toques desde Existencias`);
      expect(toques, 'toques para traspasar un artículo desde Existencias (SC-003)').toBeLessThanOrEqual(6);

      // Exactamente una pieza: ni el doble por un reintento, ni redondeos.
      await expect.poll(() => existencia(bodega.id), { message: `${bodega.name} no bajó exactamente 1` }).toBe(bodegaAntes - 1);
      expect(await existencia(venta.id), `${venta.name} no subió exactamente 1`).toBe(ventaAntes + 1);

      // Un movimiento de traspaso de cada lado, el mismo traspaso, y cada uno nombra al otro almacén.
      const salida = (await movimientos(bodega.id)).filter((m) => m.transfer_id != null && m.item_name === articulo.item_name && Number(m.quantity) === -1);
      expect(salida.length, 'la bodega no tiene el movimiento de salida del traspaso').toBeGreaterThanOrEqual(1);
      const ultima = salida.reduce((a, b) => (b.id > a.id ? b : a));
      expect(ultima.counterpart_warehouse_name, 'la salida no dice a dónde fue').toBe(venta.name);
      const entrada = (await movimientos(venta.id)).find((m) => m.transfer_id === ultima.transfer_id && m.item_name === articulo.item_name);
      expect(entrada, `${venta.name} no tiene la entrada del mismo traspaso`).toBeTruthy();
      expect(Number(entrada!.quantity), 'la entrada no es la misma cantidad que la salida').toBe(1);
      expect(entrada!.counterpart_warehouse_name, 'la entrada no dice de dónde vino').toBe(bodega.name);
      expect(entrada!.movement_type, 'los dos lados no son el mismo tipo de movimiento').toBe(ultima.movement_type);
    });
  });
}
