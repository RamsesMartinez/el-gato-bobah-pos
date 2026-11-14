import { test, expect } from '@playwright/test';
import { tokenDeApi } from './ambiente';
import { LOCAL, pedir, abrir, cerrar, entrarACaja, type Caja } from './cortes';

// CORREGIR UN CORTE CERRADO SIN REESCRIBIRLO (#290, fase 3; Maquetas M4).
//
// Antes se hacía con SQL y sin rastro. Aquí se cierra un turno con $120 de faltante, se corrige lo
// contado desde el detalle del corte, se ve lo firmado y lo corregido, y se revierte. Solo contra la
// base local: abre y cierra turnos.
test('R1 · corregir lo contado de un corte, ver el rastro y revertir', async ({ page }) => {
  test.skip(!LOCAL, 'abre y cierra turnos: solo contra la base local');
  const jwt = await tokenDeApi();
  const cajas = ((await pedir(jwt, 'GET', '/cash-registers')) as { items: Caja[] }).items;
  const caja = cajas.find((c) => !c.isPrimary && c.openSessionId === null && c.countsCash !== false);
  test.skip(!caja, 'no hay una caja secundaria cerrada');

  await abrir(jwt, caja!.id, 500, 'apertura de la suite e2e');
  const corte = await cerrar(jwt, caja!.id, 380);

  await entrarACaja(page);
  await page.goto(`/caja/historico/${corte.id}`);
  const detalle = page.getByRole('dialog').last();
  await expect(detalle.getByLabel('Diferencia: Total')).toContainText('Faltante−$120.00', { timeout: 30_000 });

  await detalle.getByRole('button', { name: 'Corregir' }).click();
  const hoja = page.getByRole('dialog').last();
  await hoja.getByRole('button', { name: 'Lo contado en el cajón' }).click();
  await hoja.getByLabel('Debió decir').fill('500');
  await hoja.getByRole('button', { name: 'Se contó mal' }).click();
  await hoja.getByLabel('Qué pasó').fill('El efectivo de un gasto estaba en el sobre (e2e)');
  await page.screenshot({ path: test.info().outputPath('r1-hoja-1024.png') });
  await hoja.getByRole('button', { name: 'Guardar corrección' }).click();

  await expect(detalle.getByText('Contado $380.00, corregido a $500.00')).toBeVisible({ timeout: 30_000 });
  await expect(detalle.getByLabel('Diferencia: Total')).toContainText('Cuadra');
  // Lo firmado sigue a la vista.
  await expect(detalle.getByLabel('Contado del cajón')).toHaveText('$380.00');
  await page.screenshot({ path: test.info().outputPath('r1-corregido-1024.png') });

  await detalle.getByRole('button', { name: 'Revertir' }).click();
  await page.getByRole('dialog').last().getByRole('textbox').fill('Me equivoqué de corte (e2e)');
  await page.getByRole('button', { name: 'Revertir la corrección' }).click();
  await expect(detalle.getByText('Revertida')).toBeVisible({ timeout: 30_000 });
  await expect(detalle.getByLabel('Diferencia: Total')).toContainText('Faltante−$120.00');
});
