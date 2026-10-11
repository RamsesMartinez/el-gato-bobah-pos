import { test, expect, type Page } from '@playwright/test';
import { API, tokenDeRequest } from './ambiente';
import { entrar, entregarPorApi, pagarPorApi, pedidoPorApi } from './pos';

// ASISTENCIA Y BITÁCORA (spec 033) contra el ambiente de pruebas.
//
// La persona que checa es «Prueba Asistencia» (cajero, PIN 4826), creada para la suite en el
// ambiente de pruebas. El test la deja con la salida checada, como la encontró si estaba fuera.

const PERSONA = 'Prueba Asistencia';
const PIN = '4826';

async function checar(page: Page, accion: 'Entrada' | 'Salida') {
  const vista = page.getByRole('dialog', { name: 'Asistencia' }).or(page.locator('main'));
  await vista.getByRole('button', { name: new RegExp(`^${PERSONA}`) }).first().click();
  const pin = page.getByRole('dialog', { name: `PIN de ${PERSONA}` });
  for (const d of PIN) await pin.getByRole('button', { name: d, exact: true }).click();
  await pin.getByRole('button', { name: accion, exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: accion })).toBeVisible();
  await page.getByRole('status').filter({ hasText: accion }).click();
}

test('checar entrada y salida tarda menos de 15 s desde que se abre la pantalla (SC-001)', async ({ page, request }) => {
  const jwt = await tokenDeRequest(request);
  const roster = await (await request.get(`${API}/attendance/roster`, { headers: { Authorization: `Bearer ${jwt}` } })).json() as
    { enabled: boolean; people: Array<{ name: string; openSince?: string }> };
  expect(roster.enabled, 'la asistencia está apagada en el ambiente de pruebas').toBe(true);
  const dentro = roster.people.find((p) => p.name === PERSONA)?.openSince !== undefined;

  await entrar(page);
  const primera: 'Entrada' | 'Salida' = dentro ? 'Salida' : 'Entrada';
  const segunda: 'Entrada' | 'Salida' = dentro ? 'Entrada' : 'Salida';

  const desde = Date.now();
  await page.getByRole('link', { name: 'Asistencia' }).first().click();
  await checar(page, primera);
  const tardo = Date.now() - desde;
  expect(tardo, `checar tardó ${tardo} ms`).toBeLessThan(15_000);

  // Al volver, la misma persona ofrece la acción contraria: el servidor ya la registró.
  await checar(page, segunda);
  // Si estaba dentro al empezar, se le deja dentro: la suite no cambia el estado de nadie.
});

test('el historial de un pedido cobrado se lee en la bitácora', async ({ page, request }) => {
  const jwt = await tokenDeRequest(request);
  const o = await pedidoPorApi(jwt);
  await pagarPorApi(jwt, o.id);
  await entregarPorApi(jwt, o.id);

  const h = await request.get(`${API}/orders/${o.id}/history`, { headers: { Authorization: `Bearer ${jwt}` } });
  expect(h.ok()).toBe(true);
  const acciones = ((await h.json()) as Array<{ action: string }>).map((e) => e.action);
  for (const a of ['order.created', 'payment.charged', 'order.delivered']) {
    expect(acciones, `al historial le falta ${a}`).toContain(a);
  }

  await entrar(page);
  await page.goto('/reportes');
  await page.getByRole('tab', { name: 'Bitácora' }).click();
  await page.getByRole('button', { name: /Entregado/ }).first().click();
  await page.getByRole('button', { name: 'Ver historial del pedido' }).click();
  const hoja = page.getByRole('dialog', { name: 'Historial del pedido' });
  await expect(hoja).toBeVisible();
  await expect(hoja.getByText('Pedido creado').first()).toBeVisible();
});
