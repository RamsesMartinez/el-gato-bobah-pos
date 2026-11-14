import { test, expect } from '@playwright/test';
import { tokenDeApi } from './ambiente';
import { detalleDelCorte } from './cierre';
import { LOCAL, pedir, abrir, cerrar, entrarACaja, type Caja } from './cortes';

// UNA SOLA DIFERENCIA, Y LA EXPLICACIÓN A LA VISTA (#290, fase 1).
//
// El caso que lo disparó: la Caja B cerró con el cajón corto, el diálogo de cierre dijo «$0», el
// Histórico dijo otra cifra, y la nota con la que se abrió el turno siguiente no aparecía en ninguna
// pantalla. Aquí se cierra un turno con $120 de faltante en una caja que no es la principal y se abre
// el siguiente con motivo; la pantalla tiene que decir lo mismo en la lista y en el detalle, y
// mostrar la explicación.
//
const NOTA = 'Un gasto salió de esta caja y se contó como si fuera de la otra (e2e)';


test('D1 · el corte con faltante dice lo mismo en la lista y en el detalle, y muestra la explicación de la apertura siguiente', async ({ page }) => {
  test.skip(!LOCAL, 'abre y cierra turnos: solo contra la base local');
  const jwt = await tokenDeApi();
  const cajas = ((await pedir(jwt, 'GET', '/cash-registers')) as { items: Caja[] }).items;
  const caja = cajas.find((c) => !c.isPrimary && c.openSessionId === null && c.countsCash !== false);
  test.skip(!caja, 'no hay una caja secundaria cerrada');

  // Turno con $500 de fondo que cierra contando $380: faltan $120 en el cajón.
  await abrir(jwt, caja!.id, 500, 'apertura de la suite e2e');
  const corte = await cerrar(jwt, caja!.id, 380);
  // El turno siguiente cuenta $500 y explica la diferencia.
  const siguiente = await abrir(jwt, caja!.id, 500, NOTA);
  try {
    await entrarACaja(page);
    await page.getByRole('tab', { name: 'Histórico' }).click();
    const fila = page.getByRole('row').filter({ hasText: caja!.name }).filter({ hasText: 'Cerrada' }).first();
    await expect(fila).toContainText('Faltante−$120.00', { timeout: 30_000 });
    await expect(fila).toContainText('Explicado al abrir');
    await fila.getByRole('button', { name: 'Ver' }).click();

    const detalle = detalleDelCorte(page);
    await expect(detalle.getByLabel('Diferencia: Total')).toContainText('Faltante−$120.00');
    await expect(detalle.getByLabel('Diferencia: Cajón (arqueo)')).toContainText('Faltante−$120.00');
    await expect(detalle.getByText(`Diferencia explicada al abrir el turno #${siguiente.id}`)).toBeVisible();
    await expect(detalle.getByText(NOTA)).toBeVisible();
    await expect(detalle.getByText('Corte #' + corte.id)).toBeVisible();
    await page.screenshot({ path: test.info().outputPath('d1-detalle-1024.png') });
  } finally {
    await cerrar(jwt, caja!.id, 500);
  }
});

test('D2 · el botón de tema cambia de claro a oscuro de un toque', async ({ page }) => {
  await entrarACaja(page);
  const html = page.locator('html');
  const antes = (await html.getAttribute('class')) ?? '';
  const boton = page.getByRole('button', { name: /^Cambiar a tema (oscuro|claro)$/ });
  await expect(boton).toBeInViewport();
  await boton.click();
  await expect.poll(async () => (await html.getAttribute('class')) ?? '').not.toBe(antes);
  await page.screenshot({ path: test.info().outputPath('d2-tema-1024.png') });
  // Lo deja como estaba: la preferencia es del aparato.
  await page.getByRole('button', { name: /^Cambiar a tema (oscuro|claro)$/ }).click();
});
