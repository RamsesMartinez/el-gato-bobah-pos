import { test, expect, type Page } from '@playwright/test';
import { API, tokenDeApi } from './ambiente';
import { entrar, entregarPorApi, pagarPorApi, pedido, pedidoPorApi } from './pos';
import { ALTO, ANCHO } from './pantalla';

// VENDER: LA COLUMNA DE CUENTAS Y LO QUE LE FALTA A CADA UNA (spec 089, maqueta B).
//
// Mide en un navegador de verdad —jsdom no resuelve el layout de Chakra— lo que el dueño eligió con
// cifras: a 1024×600 con el ticket abierto, siete cuentas a la vista (antes dos) y los productos más
// arriba; también con la franja de pedidos por aceptar y el aviso de agotado encima. Y recorre lo
// que vino a resolver: entregar una pagada y cobrar una entregada sin abrirlas.
//
// Los pedidos que crea los entrega y los cobra al terminar: un pedido de prueba abierto suma a «por
// cobrar» y bloquea el cierre de caja.

const creados: number[] = [];
let jwt = '';

test.beforeAll(async () => {
  jwt = await tokenDeApi();
  // Un producto del menú de este ambiente que se pida sin elegir opciones: el catálogo restaurado no
  // es el de las pruebas, y ningún nombre de la base es fijo.
  const menu = await (await fetch(`${API}/pos/menu`, { headers: { Authorization: `Bearer ${jwt}` } })).json() as
    { products?: Array<{ name: string; type: string; groups?: Array<{ min?: number; minSelect?: number; required?: boolean }> }> };
  const simple = (menu.products ?? []).find((x) => x.type === 'simple' && (x.groups ?? []).length === 0);
  if (!simple) throw new Error('el menú del ambiente no tiene un producto sin opciones');
  for (let i = 0; i < 8; i++) creados.push((await pedidoPorApi(jwt, simple.name)).id);
  await pagarPorApi(jwt, creados[0]); // pagada sin entregar
  await entregarPorApi(jwt, creados[1]); // entregada que debe
});

test.afterAll(async () => {
  for (const id of creados) {
    const p = await pedido(jwt, id).catch(() => null);
    if (!p || p.status === 'cancelada') continue;
    if (p.status !== 'entregada') await entregarPorApi(jwt, id).catch(() => {});
    if (Number(p.outstanding) > 0) await pagarPorApi(jwt, id).catch(() => {});
  }
});

const columna = (page: Page) => page.getByLabel('Cuentas', { exact: true });

async function abrirTicket(page: Page) {
  const mostrar = page.getByRole('button', { name: /Ver pedido|art ·/ }).first();
  if (await mostrar.isVisible().catch(() => false)) await mostrar.click();
  await expect(page.getByRole('button', { name: 'Ocultar pedido' })).toBeVisible();
}

// Las cuentas completas a la vista dentro de la columna y sin nada encima.
async function cuentasVisibles(page: Page): Promise<number> {
  return page.evaluate(() => {
    const lista = document.querySelector('[aria-label="Cuentas"]');
    if (!lista) return 0;
    const caja = lista.getBoundingClientRect();
    const tapa = document.querySelector('[data-testid="franja-de-agotado"]')?.getBoundingClientRect();
    return [...lista.querySelectorAll('[data-ficha="true"]')].filter((el) => {
      const b = el.getBoundingClientRect();
      const tapada = tapa && !(b.right <= tapa.left || b.left >= tapa.right || b.bottom <= tapa.top || b.top >= tapa.bottom);
      return b.top >= caja.top - 1 && b.bottom <= Math.min(caja.bottom, innerHeight) + 1 && !tapada;
    }).length;
  });
}

async function desdeDondeEmpiezanLosProductos(page: Page): Promise<number> {
  return page.evaluate(() => {
    const area = document.querySelector('[data-work-area]')!.getBoundingClientRect();
    return Math.round(document.querySelector('[data-testid="zona-de-productos"]')!.getBoundingClientRect().top - area.top);
  });
}

// Lo que el shell pinta cuando hay un pedido por aceptar (fila de 52 px) y un aviso de agotado
// (tarjeta flotante de 60 px). Se simula su alto: crear un pedido de plataforma pendiente de verdad
// dispararía la alarma en todas las tabletas de este ambiente.
async function ponerFranjas(page: Page) {
  await page.evaluate(() => {
    const area = document.querySelector('[data-work-area]')!;
    const fila = document.createElement('div');
    fila.setAttribute('data-testid', 'franja-de-entrantes');
    fila.style.cssText = 'height:52px;flex-shrink:0;background:#c00';
    area.parentElement!.insertBefore(fila, area);
    const aviso = document.createElement('div');
    aviso.setAttribute('data-testid', 'franja-de-agotado');
    aviso.style.cssText = 'position:fixed;top:60px;left:50%;transform:translateX(-50%);width:560px;height:60px;background:#c00;z-index:2000';
    document.body.appendChild(aviso);
  });
  await page.waitForTimeout(300);
}

for (const [w, h, minimo] of [[1024, 600, 7], [ANCHO, ALTO, 12]] as const) {
  test(`a ${w}×${h} con el ticket abierto caben al menos ${minimo} cuentas y los productos suben`, async ({ page }) => {
    await page.setViewportSize({ width: w, height: h });
    await entrar(page);
    await abrirTicket(page);
    await expect(columna(page).locator('[data-ficha="true"]').nth(7)).toBeAttached({ timeout: 30_000 });
    const total = await columna(page).locator('[data-ficha="true"]').count();
    const vis = await cuentasVisibles(page);
    const desde = await desdeDondeEmpiezanLosProductos(page);
    console.log(`[e2e] ${w}×${h}: ${vis} de ${total} cuentas a la vista; productos desde y=${desde}`);
    expect(vis, 'caben menos cuentas que en la maqueta elegida').toBeGreaterThanOrEqual(Math.min(minimo, total));
    expect(desde, 'los productos bajaron: antes empezaban en 196').toBeLessThan(160);

    // Ningún control del encabezado ni de la columna mide menos de 44 px.
    const chicos = await page.evaluate(() => [...document.querySelectorAll('[data-work-area] button, [data-work-area] input')]
      .filter((e) => !e.closest('[data-testid="zona-de-productos"]'))
      .filter((e) => { const b = e.getBoundingClientRect(); return b.width > 0 && b.height > 0 && b.height < 43.5; })
      .map((e) => e.getAttribute('aria-label') || e.textContent));
    expect(chicos, 'controles de menos de 44 px').toEqual([]);

    // Cada renglón de la columna mide lo de un renglón: «Cuenta nueva» llegó a estirarse a todo el
    // alto de la columna, y con ella sola no se veía otra cosa.
    const altos = await columna(page).locator('[data-ficha="true"]').evaluateAll((els) => els.map((e) => Math.round(e.getBoundingClientRect().height)));
    expect(Math.max(...altos), `un renglón de la columna mide ${Math.max(...altos)} px`).toBeLessThan(90);

    // Con la franja de pedidos por aceptar y el aviso de agotado: la cuenta abierta y una fila
    // completa de productos se siguen viendo, sin nada encima.
    await ponerFranjas(page);
    const conFranjas = await cuentasVisibles(page);
    const fila = await page.evaluate(() => {
      const zona = document.querySelector('[data-testid="zona-de-productos"]')!.getBoundingClientRect();
      const primero = document.querySelector('[data-testid="zona-de-productos"] button')?.getBoundingClientRect();
      return !!primero && primero.top >= zona.top - 1 && primero.bottom <= Math.min(zona.bottom, innerHeight) + 1;
    });
    console.log(`[e2e] ${w}×${h} con franjas: ${conFranjas} cuentas a la vista`);
    expect(conFranjas, 'con las franjas no queda ni una cuenta a la vista').toBeGreaterThan(0);
    expect(fila, 'con las franjas no queda una fila completa de productos').toBe(true);
  });
}

test('sin cuentas, «Cuenta nueva» mide lo de un renglón', async ({ page }) => {
  await page.setViewportSize({ width: 1024, height: 600 });
  await entrar(page);
  const nueva = columna(page).getByRole('button', { name: 'Cuenta nueva, sin productos' });
  await expect(nueva).toBeVisible();
  expect((await nueva.boundingBox())!.height, '«Cuenta nueva» se estira a todo el alto de la columna').toBeLessThan(90);
});

test('«Entregar» en la columna entrega la pagada y la quita de la fila', async ({ page }) => {
  await page.setViewportSize({ width: 1024, height: 600 });
  await entrar(page);
  const p = await pedido(jwt, creados[0]);
  const boton = page.getByRole('button', { name: `Entregar ${p.folioName}` });
  await boton.scrollIntoViewIfNeeded();
  await boton.click();
  await expect.poll(async () => (await pedido(jwt, creados[0])).status, { timeout: 15_000 }).toBe('entregada');
  await expect(columna(page).getByRole('button', { name: new RegExp(`^${p.folioName} ·`) })).toHaveCount(0);
});

test('«Cobrar» en la columna abre el cobro de la entregada que debe', async ({ page }) => {
  await page.setViewportSize({ width: 1024, height: 600 });
  await entrar(page);
  const p = await pedido(jwt, creados[1]);
  const boton = page.getByRole('button', { name: new RegExp(`^Cobrar .* ${p.folioName}$`) });
  await boton.scrollIntoViewIfNeeded();
  await boton.click();
  const hoja = page.getByRole('dialog');
  await expect(hoja).toBeVisible({ timeout: 15_000 });
  await expect(hoja.getByRole('button', { name: 'Efectivo' })).toBeVisible({ timeout: 15_000 });
  const caja = await hoja.boundingBox();
  expect(caja!.height, 'la hoja de cobro no cabe en la pantalla').toBeLessThanOrEqual(600);
});
