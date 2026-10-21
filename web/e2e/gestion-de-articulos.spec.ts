import { test, expect, type Page } from '@playwright/test';
import { entrar } from './pos';

// GESTIÓN DE ARTÍCULOS (spec 042, T045; escrita en la 043): lo que jsdom no puede medir. Cuántos
// renglones de la lista caben a 1024×600, que todo lo tocable mida 44 px y que la ficha no se salga
// de lado, con la tableta acostada y de pie. Solo lee: no crea ni edita artículos.

test.use({ hasTouch: true });

const CAPTURAS = process.env.E2E_CAPTURAS ?? 'test-results/gestion-de-articulos';

async function abrirArticulos(page: Page) {
  await entrar(page);
  await page.goto('/almacen');
  await page.getByRole('tab', { name: 'Artículos' }).click();
  await expect(page.locator('tr[role="button"]').first()).toBeVisible({ timeout: 15_000 });
}

// chicos devuelve los controles visibles que miden menos de 44 px de alto, con su texto: el
// mensaje del fallo dice cuál es, no solo cuántos.
async function chicos(page: Page) {
  return page.evaluate(() => [...document.querySelectorAll('button,input,[role=button],[role=tab],[role=switch],textarea')]
    .filter((e) => {
      const r = e.getBoundingClientRect();
      const s = getComputedStyle(e);
      return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && r.bottom > 0 && r.top < innerHeight;
    })
    .map((e) => ({ alto: Math.round(e.getBoundingClientRect().height), texto: ((e as HTMLElement).innerText || (e as HTMLInputElement).placeholder || e.getAttribute('aria-label') || '').trim().slice(0, 40) }))
    // El número de versión del pie es texto, no un control que se toque.
    .filter((c) => c.alto < 44 && !/^v[0-9a-f]{7}$/.test(c.texto)));
}

const deLado = (page: Page) => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);

test('la lista muestra 6 renglones completos y todo lo tocable mide 44 px', async ({ page }) => {
  await abrirArticulos(page);
  await page.screenshot({ path: `${CAPTURAS}/lista.png` });
  const completos = await page.evaluate(() => {
    const filas = [...document.querySelectorAll('tr[role="button"]')];
    let tope = innerHeight;
    for (let e = filas[0]?.parentElement; e && e !== document.body; e = e.parentElement) {
      const s = getComputedStyle(e);
      if (s.overflowY === 'auto' || s.overflowY === 'scroll') { tope = Math.min(tope, e.getBoundingClientRect().bottom); break; }
    }
    return filas.filter((f) => { const b = f.getBoundingClientRect(); return b.top >= 0 && b.bottom <= tope + 0.5; }).length;
  });
  console.log(`[e2e] renglones completos a 1024×600: ${completos}`);
  expect(completos, 'renglones de la lista que se ven completos a 1024×600').toBeGreaterThanOrEqual(6);
  expect(await chicos(page)).toEqual([]);
  expect(await deLado(page), 'la lista se desplaza de lado').toBeLessThanOrEqual(0);
});

for (const [nombre, viewport] of [['de lado', { width: 1024, height: 600 }], ['de pie', { width: 600, height: 1024 }]] as const) {
  test(`la ficha cabe ${nombre} sin desplazarse de lado y sus controles miden 44 px`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await abrirArticulos(page);
    await page.locator('tr[role="button"]').first().click();
    await expect(page.getByRole('button', { name: '‹ Artículos' })).toBeVisible();
    await page.screenshot({ path: `${CAPTURAS}/ficha-${nombre.replace(' ', '-')}.png` });
    expect(await chicos(page)).toEqual([]);
    expect(await deLado(page), `la ficha ${nombre} se desplaza de lado`).toBeLessThanOrEqual(0);
  });
}
