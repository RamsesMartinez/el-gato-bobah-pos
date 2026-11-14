import { test, expect, type Page } from '@playwright/test';
import { API, tokenDeApi } from './ambiente';
import { iniciarSesion } from './sesion';
import { marcarEntrada, marcarSalida } from './asistencia';

// UNA SOLA DIFERENCIA, Y LA EXPLICACIÓN A LA VISTA (#290, fase 1).
//
// El caso que lo disparó: la Caja B cerró con el cajón corto, el diálogo de cierre dijo «$0», el
// Histórico dijo otra cifra, y la nota con la que se abrió el turno siguiente no aparecía en ninguna
// pantalla. Aquí se cierra un turno con $120 de faltante en una caja que no es la principal y se abre
// el siguiente con motivo; la pantalla tiene que decir lo mismo en la lista y en el detalle, y
// mostrar la explicación.
//
// Solo contra la base local: abre y cierra turnos, y en un ambiente compartido serían de alguien.
const LOCAL = /^https?:\/\/(localhost|127\.0\.0\.1)[:/]/.test(API);

async function pedir(jwt: string, metodo: string, ruta: string, cuerpo?: unknown) {
  const r = await fetch(`${API}${ruta}`, {
    method: metodo, headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' },
    body: cuerpo === undefined ? undefined : JSON.stringify(cuerpo),
  });
  if (!r.ok) throw new Error(`${metodo} ${ruta}: ${r.status} ${await r.text()}`);
  return r.json();
}

type Caja = { id: number; name: string; isPrimary: boolean; openSessionId: number | null; countsCash?: boolean };
const NOTA = 'Un gasto salió de esta caja y se contó como si fuera de la otra (e2e)';

async function cerrar(jwt: string, registerId: number, contado: number) {
  await marcarSalida(jwt);
  const turno = (await pedir(jwt, 'GET', `/cash-sessions/current?registerId=${registerId}`)) as
    { terminalsToCount?: Array<{ terminalId: number }> };
  const terminalCounts = Object.fromEntries((turno.terminalsToCount ?? []).map((t) => [String(t.terminalId), 0]));
  return pedir(jwt, 'POST', '/cash-sessions/close', {
    registerId, countedCash: contado, manualReason: 'cierre de la suite e2e', tipsDecision: 'quedan_en_caja', terminalCounts,
  }) as Promise<{ id: number }>;
}

async function abrir(jwt: string, registerId: number, fondo: number, nota: string) {
  await marcarEntrada(jwt);
  return pedir(jwt, 'POST', '/cash-sessions', {
    registerId, openingCash: fondo, manualReason: 'apertura de la suite e2e',
    openingReason: 'last_count_wrong', openingReasonNote: nota,
  }) as Promise<{ id: number }>;
}

async function entrarACaja(page: Page) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  await expect(page.getByRole('button', { name: 'Cuenta nueva', exact: true })).toBeVisible({ timeout: 30_000 });
  await page.goto('/caja');
  await page.waitForLoadState('networkidle');
}

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

    const detalle = page.getByRole('dialog').last();
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
