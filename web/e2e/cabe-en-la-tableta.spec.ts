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

// X7 · LOS CONTROLES DEL RENGLÓN DEL TICKET, MEDIDOS EN PÍXELES REALES.
//
// Medían ~24 px, por debajo del piso de 44 que fija la constitución. Vitest no puede atrapar esto:
// las medidas de Chakra son clases CSS y jsdom no las resuelve, así que un assert de píxeles allá
// pasaría verde con los botones chicos. Aquí hay un navegador de verdad.
//
// Y se mide también la SEPARACIÓN con la papelera: estaba pegada al menos, así que quitar una
// unidad y borrar el renglón entero se distinguían por unos pocos píxeles.
test('X7 · los controles del renglón del ticket miden 44 px y la papelera va aparte', async ({ page }) => {
  await entrar(page);
  await page.getByText('Dedos de Queso Pza').first().click();

  const menos = page.getByRole('button', { name: '−' }).first();
  const mas = page.getByRole('button', { name: '+' }).first();
  const quitar = page.getByRole('button', { name: 'Quitar' }).first();

  for (const [nombre, boton] of [['−', menos], ['+', mas], ['Quitar', quitar]] as const) {
    const caja = await boton.boundingBox();
    expect(caja, `no se encontró el control "${nombre}"`).not.toBeNull();
    expect(caja!.height, `"${nombre}" mide ${caja!.height}px de alto y el piso es 44`).toBeGreaterThanOrEqual(44);
    expect(caja!.width, `"${nombre}" mide ${caja!.width}px de ancho y el piso es 44`).toBeGreaterThanOrEqual(44);
  }

  // La papelera al otro extremo: entre ella y el "+" tiene que haber más que el hueco de un gap.
  const cajaMas = (await mas.boundingBox())!;
  const cajaQuitar = (await quitar.boundingBox())!;
  const hueco = cajaQuitar.x - (cajaMas.x + cajaMas.width);
  expect(hueco, `la papelera está a ${Math.round(hueco)}px del "+": un toque impreciso borra el renglón`)
    .toBeGreaterThan(40);
});
