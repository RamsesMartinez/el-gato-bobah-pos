import { expect, type Page } from '@playwright/test';

// El cierre de caja es un asistente de cuatro pasos desde la spec 082 (#290, Maquetas M3 B): las
// pruebas que cuentan, capturan o cierran entran a él y van al paso que les toca.
export type PasoDelCierre = 'Pendientes' | 'Cajón' | 'Otros métodos' | 'Diferencia y cierre';

export async function alPasoDelCierre(page: Page, paso: PasoDelCierre) {
  const pasos = page.getByRole('navigation', { name: 'Pasos del cierre' });
  if (!(await pasos.isVisible().catch(() => false))) {
    await page.getByRole('button', { name: 'Cerrar caja' }).first().click();
  }
  await expect(pasos).toBeVisible({ timeout: 30_000 });
  await pasos.getByRole('button', { name: new RegExp(paso) }).click();
}

// El detalle de un corte a 1024 px o más: el panel al lado de la lista (#290, Maquetas M1 A). Por
// nombre y no como «el último diálogo»: la hoja de corregir también es un diálogo y lo tapaba.
export function detalleDelCorte(page: Page) {
  return page.getByRole('region', { name: 'Detalle del corte' });
}

// contarEnElCierre cuenta el cajón en el paso 2 con las piezas dadas (etiqueta → cuántas).
export async function contarEnElCierre(page: Page, piezas: Record<string, string>) {
  await alPasoDelCierre(page, 'Cajón');
  await page.getByRole('button', { name: /^(Contar efectivo|Volver a contar)$/ }).click();
  const [primera] = Object.keys(piezas);
  await expect(page.getByLabel(primera, { exact: true })).toBeVisible({ timeout: 30_000 });
  for (const [etiqueta, n] of Object.entries(piezas)) await page.getByLabel(etiqueta, { exact: true }).fill(n);
  await page.getByRole('button', { name: 'Usar este conteo' }).click();
}

// cerrarConElAsistente termina el cierre desde el paso en que esté: la propina heredada se queda en
// caja (no es de la prueba), todo el efectivo se queda en el cajón, y se confirma en la hoja.
export async function cerrarConElAsistente(page: Page, avisoAlConfirmar?: string) {
  await alPasoDelCierre(page, 'Pendientes');
  const propina = page.getByRole('button', { name: 'Se queda en caja' });
  if (await propina.isVisible().catch(() => false)) await propina.click();
  await alPasoDelCierre(page, 'Diferencia y cierre');
  const no = page.getByRole('button', { name: 'No, todo se queda' });
  if (await no.isVisible().catch(() => false)) await no.click();
  const cerrar = page.getByRole('button', { name: 'Cerrar caja' });
  await expect(cerrar).toBeEnabled({ timeout: 30_000 });
  await cerrar.click();
  if (avisoAlConfirmar) await expect(page.getByRole('dialog').last().getByText(avisoAlConfirmar)).toBeVisible();
  await page.getByRole('dialog').last().getByRole('button', { name: 'Cerrar caja' }).click();
}
