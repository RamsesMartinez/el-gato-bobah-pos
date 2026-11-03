import { test, expect } from '@playwright/test';
import { API, tokenDeRequest } from './ambiente';
import { iniciarSesion } from './sesion';

// LA MISMA EXISTENCIA SE DICE IGUAL EN LAS TRES PANTALLAS (spec 045, SC-001).
//
// Existencias decía «-3000 ml» y Artículos «-3 L» para el mismo artículo, y «1 pieza» contra «1
// piezas». Aquí se recorre CADA artículo activo de la base: su texto de existencia en Existencias, en
// Artículos y en su ficha tiene que ser idéntico. Solo lee. Con la base restaurada son cientos de
// fichas, así que va despacio a propósito: la ficha es la pantalla que más se abre al revisar.

test.use({ hasTouch: true });

const limpio = (s: string) => s.replace(/bajo|Revisar|·/g, '').replace(/\s+/g, ' ').trim();

test('SC-001 · la existencia de cada artículo dice lo mismo en Existencias, Artículos y la ficha', async ({ page, request }) => {
  test.setTimeout(15 * 60_000);
  const jwt = await tokenDeRequest(request);
  const r = await request.get(`${API}/stock/articles`, { headers: { Authorization: `Bearer ${jwt}` } });
  expect(r.ok()).toBe(true);
  const articulos = (await r.json()) as { id: number; name: string }[];
  expect(articulos.length, 'no hay artículos que comparar: ¿se restauró la base?').toBeGreaterThan(0);

  await page.goto('/');
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);

  await page.goto('/almacen?tab=existencias');
  await expect(page.locator('table tbody tr').first()).toBeVisible({ timeout: 30_000 });
  const existencias = new Map<string, string>(await page.locator('table tbody tr').evaluateAll((rows) =>
    rows.filter((r) => r.children.length >= 3 && r.children[1].textContent === 'insumo')
      .map((r) => [r.children[0].textContent ?? '', r.children[2].textContent ?? ''] as [string, string])));

  await page.goto('/almacen?tab=insumos');
  await expect(page.getByRole('button', { name: articulos[0].name }).first()).toBeVisible({ timeout: 30_000 });
  const lista = new Map<string, string>(await page.locator('tr[role="button"]').evaluateAll((rows) =>
    rows.map((r) => [r.getAttribute('aria-label') ?? '', r.children[1]?.textContent ?? ''] as [string, string])));

  // La ficha se abre tocando el renglón, como quien opera: recargar la página por cada una (cientos)
  // pide un refresco de sesión por carga y se topa con el limitador.
  const distintos: string[] = [];
  let fichas = 0;
  for (const a of articulos) {
    const renglon = page.getByRole('button', { name: a.name, exact: true }).first();
    if (!(await renglon.isVisible().catch(() => false))) continue;
    await renglon.click();
    const hay = page.getByText(/Hay -?[\d.,]+ \S+/).first();
    await expect(hay).toBeVisible({ timeout: 30_000 });
    const ficha = ((await hay.textContent()) ?? '').match(/Hay (-?[\d.,]+ \S+)/)![1];
    fichas++;
    await page.getByRole('button', { name: 'Volver a Artículos' }).click();
    await expect(hay).toBeHidden({ timeout: 30_000 });
    const enExistencias = existencias.has(a.name) ? limpio(existencias.get(a.name)!) : null;
    const enLista = lista.has(a.name) ? limpio(lista.get(a.name)!) : null;
    const textos = [enExistencias, enLista, ficha].filter((t): t is string => t !== null);
    if (new Set(textos).size > 1) distintos.push(`${a.name}: Existencias «${enExistencias}» · Artículos «${enLista}» · ficha «${ficha}»`);
  }
  expect(fichas, 'no se abrió ninguna ficha').toBeGreaterThan(0);
  console.log(`[e2e] ${articulos.length} artículos, ${fichas} fichas abiertas, comparados en tres pantallas; ${distintos.length} distintos.`);
  expect(distintos, `${distintos.length} artículos dicen su existencia distinto según la pantalla`).toEqual([]);
});
