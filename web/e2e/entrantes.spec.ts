import { test, expect, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';

import { API, EMPRESA, tokenDeRequest } from './ambiente';
import { iniciarSesion } from './sesion';

// LA ALERTA DE PEDIDO ENTRANTE (spec 093), CONTRA LA API LOCAL DE VERDAD.
//
// Solo corre en local: los pedidos se siembran directo en la base del worktree
// (scripts/entrantes-de-prueba.sh) y aceptar o rechazar necesita la API con
// PLATFORM_ORDERS_SIMULATED=true, que no arranca fuera de desarrollo. Contra el ambiente de pruebas
// no hay cómo crear un pedido de plataforma, y la suite lo salta.
const LOCAL = /localhost|127\.0\.0\.1/.test(API);

function sembrar(n: number) {
  execFileSync('bash', [resolve(process.cwd(), '../scripts/entrantes-de-prueba.sh'), String(n), EMPRESA], { stdio: 'pipe' });
}

async function entrar(page: Page, ruta: string) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  await page.goto(ruta);
  await page.waitForLoadState('networkidle');
}

const franja = (page: Page) => page.getByTestId('franja-de-entrantes');

// Lo que queda pendiente de una corrida anterior se rechaza al empezar: si no, la cuenta de cada
// prueba depende de la anterior.
async function limpiarPendientes(page: Page) {
  const jwt = await tokenDeRequest(page.request);
  const r = await page.request.get(`${API}/orders/incoming`, { headers: { Authorization: `Bearer ${jwt}` } });
  for (const p of (await r.json()).orders ?? []) {
    await page.request.post(`${API}/orders/incoming/${p.source}/${p.id}/reject`,
      { headers: { Authorization: `Bearer ${jwt}` }, data: { reason: 'other' } });
  }
}

// LA PRUEBA CIERRA LO QUE CREA. La limpieza global se salta los pedidos con nombre de cliente (los
// toma por de otra persona), y uno aceptado de plataforma siempre trae uno: sin esto quedaban
// abiertos, sumaban en la barra y bloqueaban el cierre de caja.
const aceptados = new Set<number>();
function anotarAceptados(page: Page) {
  page.on('response', async (r) => {
    if (r.request().method() === 'POST' && /\/orders\/incoming\/[^/]+\/\d+\/accept$/.test(r.url()) && r.ok()) {
      aceptados.add(((await r.json()) as { id: number }).id);
    }
  });
}

test.describe('093 · pedidos por aceptar', () => {
  test.skip(!LOCAL, 'solo contra la API local con la simulación de la plataforma');
  test.beforeEach(async ({ page }) => {
    anotarAceptados(page);
    await limpiarPendientes(page);
  });
  test.afterEach(async ({ page }) => {
    await limpiarPendientes(page);
    const jwt = await tokenDeRequest(page.request);
    // Ya vienen pagados por la plataforma: entregarlos los cierra.
    for (const id of aceptados) {
      const r = await page.request.post(`${API}/orders/${id}/deliver`, { headers: { Authorization: `Bearer ${jwt}` }, data: {} });
      if (r.ok()) aceptados.delete(id);
    }
    expect([...aceptados], 'pedidos aceptados que no se pudieron entregar').toEqual([]);
  });

  test('suena en otra pantalla, lleva a Pedidos, se acepta y se rechaza con motivo', async ({ page }) => {
    await entrar(page, '/almacen');
    sembrar(2);
    // El sondeo de abajo lo trae aunque no haya aviso (la siembra no pasa por la API).
    await expect(franja(page)).toBeVisible({ timeout: 20_000 });
    await expect(franja(page)).toContainText('2 pedidos por aceptar');
    // La alarma está pidiendo sonar: sonando o, si el navegador no dejó, bloqueada con su botón.
    expect(['sonando', 'bloqueada']).toContain(await franja(page).getAttribute('data-alarma'));

    await page.getByRole('button', { name: 'Ver pedidos' }).click();
    await expect(page).toHaveURL(/\/pedidos/);
    const columna = page.locator('[data-por-aceptar]');
    await expect(columna).toBeVisible();
    // En Pedidos la columna ya lo dice: la franja se quita.
    await expect(franja(page)).toHaveCount(0);
    const alto = await columna.getByRole('button', { name: 'Aceptar' }).first().boundingBox();
    expect(alto!.height).toBeGreaterThanOrEqual(44);

    await columna.getByRole('button', { name: 'Aceptar' }).first().click();
    await expect(page.getByText(/Pedido #\d+ aceptado/)).toBeVisible();
    await expect(columna.locator('[data-entrante]')).toHaveCount(1);

    await columna.getByRole('button', { name: 'Rechazar' }).click();
    await page.getByRole('radio', { name: 'Se acabó un producto' }).click();
    await page.getByRole('button', { name: 'Rechazar pedido' }).click();
    await expect(page.getByText('Pedido rechazado')).toBeVisible();
    await expect(page.locator('[data-por-aceptar]')).toHaveCount(0);
  });

  // VARIAS TABLETAS: aceptar en una apaga la otra en menos de 5 s (SC-002).
  test('aceptar en una tableta apaga la otra', async ({ browser }) => {
    const a = await (await browser.newContext({ hasTouch: true })).newPage();
    anotarAceptados(a);
    const b = await (await browser.newContext({ hasTouch: true })).newPage();
    await limpiarPendientes(a);
    await entrar(a, '/pedidos');
    await entrar(b, '/almacen');
    sembrar(1);
    await expect(a.locator('[data-por-aceptar]')).toBeVisible({ timeout: 20_000 });
    await expect(franja(b)).toBeVisible({ timeout: 20_000 });
    await a.locator('[data-por-aceptar]').getByRole('button', { name: 'Aceptar' }).click();
    await expect(franja(b)).toHaveCount(0, { timeout: 5_000 });
  });

  // RED CAÍDA Y RECONECTADA: lo que llegó sin red suena al volver (SC-003).
  test('un pedido que llega sin red suena al volver la red', async ({ page, context }) => {
    await entrar(page, '/almacen');
    await context.setOffline(true);
    sembrar(1);
    await page.waitForTimeout(2_000);
    await expect(franja(page)).toHaveCount(0);
    await context.setOffline(false);
    await expect(franja(page)).toBeVisible({ timeout: 20_000 });
  });

  // RECARGA: lo pendiente vuelve a estar en cuanto carga.
  test('recargar no apaga la alerta', async ({ page }) => {
    await entrar(page, '/almacen');
    sembrar(1);
    await expect(franja(page)).toBeVisible({ timeout: 20_000 });
    await page.reload();
    await page.waitForLoadState('networkidle');
    await expect(franja(page)).toBeVisible({ timeout: 10_000 });
  });
});
