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
    await expect(page.getByRole('button', { name: /Volver a Artículos/ })).toBeVisible();
    await page.screenshot({ path: `${CAPTURAS}/ficha-${nombre.replace(' ', '-')}.png` });
    expect(await chicos(page)).toEqual([]);
    expect(await deLado(page), `la ficha ${nombre} se desplaza de lado`).toBeLessThanOrEqual(0);
  });
}

// Hoja de empaque nuevo (spec 043, R5): con el teclado abierto la tableta deja ~300 px de alto. La
// frase de lo que se guarda y «Guardar empaque» tienen que quedar a la vista. El teclado se simula
// encogiendo la ventana (Chromium no pinta el del sistema). No guarda nada: cancela al final.
test('la hoja de empaque deja la frase y «Guardar empaque» a la vista con el teclado abierto', async ({ page }) => {
  await abrirArticulos(page);
  await page.locator('tr[role="button"]').first().click();
  await page.getByRole('button', { name: '+ Agregar empaque' }).click();
  const hoja = page.locator('[role="dialog"]').last();
  await expect(hoja.getByText('¿En qué empaque lo compras?')).toBeVisible();
  const primero = hoja.locator('[data-packs] button').first();
  const nombre = (await primero.innerText()).trim();
  await primero.click();
  await hoja.getByLabel('Cuánto trae').fill('5');
  await page.screenshot({ path: `${CAPTURAS}/empaque.png` });
  expect(await chicos(page)).toEqual([]);
  await page.setViewportSize({ width: 1024, height: 300 });
  await hoja.getByLabel('Cuánto trae').focus();
  await page.screenshot({ path: `${CAPTURAS}/empaque-con-teclado.png` });
  await expect(hoja.getByText(new RegExp(`^1 ${nombre.toLowerCase()} = 5 `))).toBeInViewport();
  await expect(hoja.getByRole('button', { name: 'Guardar empaque' })).toBeInViewport();
  await hoja.getByRole('button', { name: 'Cancelar' }).click();
});

// Límites del conteo (spec 044, T038): controles de 44 px y «Guardar límites» a la vista con el
// teclado del sistema abierto (ventana encogida a 350 px). Escribe sin guardar y descarta.
test('los límites del conteo dejan «Guardar límites» a la vista con el teclado abierto', async ({ page }) => {
  await entrar(page);
  await page.goto('/negocio');
  const seccion = page.getByTestId('limites-conteo');
  await seccion.scrollIntoViewIfNeeded({ timeout: 15_000 });
  const bajos = await seccion.locator('input, button').evaluateAll((els) => els
    .map((e) => Math.round(e.getBoundingClientRect().height)).filter((h) => h > 0 && h < 44));
  expect(bajos, 'controles de menos de 44 px').toEqual([]);
  await page.setViewportSize({ width: 1024, height: 350 });
  const campo = seccion.getByLabel('Máximo por conteo de Peso (kg)');
  const antes = await campo.inputValue();
  await campo.fill('999');
  await page.screenshot({ path: `${CAPTURAS}/limites-con-teclado.png` });
  await expect(campo).toBeInViewport();
  await expect(seccion.getByRole('button', { name: 'Guardar límites' })).toBeInViewport();
  await seccion.getByRole('button', { name: 'Descartar' }).click();
  await expect(campo).toHaveValue(antes);
});
