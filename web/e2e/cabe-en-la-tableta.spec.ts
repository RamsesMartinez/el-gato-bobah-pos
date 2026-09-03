import { test, expect, type Page } from '@playwright/test';

// LA HOJA DE COBRO TIENE QUE CABER EN LA TABLETA.
//
// El presupuesto real es 1024x600 y el alto es lo que escasea: cada control que se agrega a esta
// hoja se lo quita a lo que el operador vino a leer. Es un requisito funcional del producto y se
// olvida solo, así que se mide en vez de recordarse.
//
// Aquí se mide lo que NO se puede medir en vitest: el alto que ocupan de verdad los métodos de pago
// del negocio, que son tantos como ese negocio tenga configurados.

const USUARIO = process.env.E2E_USER ?? 'admin';
const EMPRESA = process.env.E2E_SLUG ?? 'gatobobah';
const PASSWORD = process.env.E2E_PASSWORD ?? 'Dev-ffb903b3dfb31073!';

async function entrar(page: Page) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  const usuario = page.getByPlaceholder('usuario@empresa');
  if (await usuario.isVisible().catch(() => false)) {
    await usuario.fill(`${USUARIO}@${EMPRESA}`);
    await page.getByPlaceholder('Contraseña').fill(PASSWORD);
    await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  }
  await expect(page.getByRole('button', { name: 'Cuenta 1' })).toBeVisible({ timeout: 30_000 });
}

test('E7 · la hoja de cobro cabe en 600 px, con y sin repartir', async ({ page }) => {
  await entrar(page);
  await page.getByText('Dedos de Queso Pza').first().click();
  const confirmar = page.getByRole('button', { name: /^(Agregar|Confirmar)/ });
  if (await confirmar.isVisible().catch(() => false)) await confirmar.click();
  const pildora = page.getByRole('button', { name: /art ·/ });
  if (await pildora.isVisible().catch(() => false)) await pildora.click();
  await page.getByRole('button', { name: 'COBRAR' }).click();

  // Se espera a los métodos de pago: la hoja crece cuando llegan, y medir antes da un alto que el
  // operador nunca ve.
  await expect(page.getByRole('button', { name: 'Efectivo' })).toBeVisible({ timeout: 30_000 });
  const alto = async () => Math.round(
    (await page.locator('[role="dialog"]').first().boundingBox())?.height ?? 0);

  const sinRepartir = await alto();
  expect(sinRepartir, 'la hoja no cabe en la tableta sin repartir').toBeLessThanOrEqual(600);

  await page.getByRole('button', { name: /Dividir/ }).click();
  const repartiendo = await alto();
  expect(repartiendo, 'la hoja no cabe en la tableta al repartir').toBeLessThanOrEqual(600);

  // EL REPARTIDOR NO ES GRATIS, y por eso no vive abierto.
  //
  // Antes eran cuatro botones fijos —Todo, entre 2, 3 y 4— presentes en TODO cobro, y casi todos
  // los pedidos se le cobran a una sola persona. Esta diferencia es lo que la hoja dejó de pagar
  // en el caso común; si algún día vuelve a cero, es que el repartidor volvió a estar siempre.
  expect(repartiendo - sinRepartir,
    'el repartidor dejó de costar alto: ¿volvió a estar siempre abierto?').toBeGreaterThan(40);
  console.log(`[e2e] hoja de cobro: ${sinRepartir}px sin repartir, ${repartiendo}px repartiendo, de 600px`);
});
