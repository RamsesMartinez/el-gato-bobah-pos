import { test, expect, type Page } from '@playwright/test';

// (sin API: la medición pasa por la pantalla)

async function entrar(page: Page) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  const usuario = page.getByPlaceholder('usuario@empresa');
  if (await usuario.isVisible().catch(() => false)) {
    await usuario.fill('admin@gatobobah');
    await page.getByPlaceholder('Contraseña').fill('Dev-ffb903b3dfb31073!');
    await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  }
  await expect(page.getByRole('button', { name: 'Cuenta 1' })).toBeVisible({ timeout: 30_000 });
}

test('medir la hoja de cobro a 1024x600', async ({ page }) => {
  await entrar(page);
  await page.getByText('Dedos de Queso Pza').first().click();
  const confirmar = page.getByRole('button', { name: /^(Agregar|Confirmar)/ });
  if (await confirmar.isVisible().catch(() => false)) await confirmar.click();
  const pildora = page.getByRole('button', { name: /art ·/ });
  if (await pildora.isVisible().catch(() => false)) await pildora.click();
  await page.getByRole('button', { name: 'COBRAR' }).click();
  await expect(page.getByText(/Falta \$/)).toBeVisible({ timeout: 30_000 });

  // La distancia entre el rótulo del reparto y el siguiente bloque ES lo que cuesta el reparto.
  const y = async (texto: string) => {
    const t = page.getByText(texto).first();
    if (!(await t.count())) return null;
    const b = await t.boundingBox();
    return b ? Math.round(b.y) : null;
  };
  const hoja = await page.locator('[role="dialog"]').first().boundingBox();
  const yReparto = await y('¿Cuánto cobras ahora?');
  const yMetodos = await y('¿Con qué paga?');
  const cuesta = yReparto !== null && yMetodos !== null ? yMetodos - yReparto : 0;
  console.log(`MEDIDO hoja=${Math.round(hoja?.height ?? 0)}px de 600 | el reparto cuesta ${cuesta}px `
    + `(de y=${yReparto} a y=${yMetodos})`);
});
