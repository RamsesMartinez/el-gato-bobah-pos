import { test, expect, type Page } from '@playwright/test';
import { API, tokenDeRequest } from './ambiente';
import { entrar, entregarPorApi, pagarPorApi, pedidoPorApi } from './pos';
import { abrirDelMenu } from './menu';
import { altaDePrueba, bajaDePrueba, type PersonaDePrueba } from './personas';

// ASISTENCIA Y BITÁCORA (spec 033).
//
// La persona que checa la crea la prueba y la da de baja al final (personas.ts): la que antes se
// daba por hecha, «Prueba Asistencia», solo existía en el ambiente de pruebas, y contra una base
// restaurada la prueba fallaba buscándola.

async function checar(page: Page, p: PersonaDePrueba, accion: 'Entrada' | 'Salida') {
  const vista = page.getByRole('dialog', { name: 'Asistencia' }).or(page.locator('main'));
  await vista.getByRole('button', { name: new RegExp(`^${escapar(p.name)}`) }).first().click();
  const pin = page.getByRole('dialog', { name: `PIN de ${p.name}` });
  for (const d of p.pin) await pin.getByRole('button', { name: d, exact: true }).click();
  await pin.getByRole('button', { name: accion, exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: accion })).toBeVisible();
  await page.getByRole('status').filter({ hasText: accion }).click();
}

const escapar = (t: string) => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

test('checar entrada y salida tarda menos de 15 s desde que se abre la pantalla (SC-001)', async ({ page, request }) => {
  const jwt = await tokenDeRequest(request);
  const roster = await (await request.get(`${API}/attendance/roster`, { headers: { Authorization: `Bearer ${jwt}` } })).json() as
    { enabled: boolean };
  expect(roster.enabled, 'la asistencia está apagada en esta empresa').toBe(true);
  // Nombre propio por corrida: con uno fijo, una corrida interrumpida deja una persona activa con el
  // mismo nombre y la siguiente toca a esa, con otro PIN.
  const persona = await altaDePrueba(jwt, `Asistencia-${Date.now().toString(36)}`, 'cajero');
  try {
    await entrar(page);
    const desde = Date.now();
    await abrirDelMenu(page, 'Asistencia');
    await checar(page, persona, 'Entrada');
    const tardo = Date.now() - desde;
    expect(tardo, `checar tardó ${tardo} ms`).toBeLessThan(15_000);
    // Al volver, la misma persona ofrece la acción contraria: el servidor ya la registró.
    await checar(page, persona, 'Salida');
  } finally {
    // Si falló con la entrada checada, la cierra antes de la baja: dada de baja ya no sale en la lista
    // para checar y su turno quedaría abierto para siempre. Si ya salió, el servidor lo rechaza y
    // no pasa nada.
    await request.post(`${API}/attendance/clock`, {
      headers: { Authorization: `Bearer ${jwt}` }, data: { userId: persona.id, pin: persona.pin, expected: 'out' },
    });
    await bajaDePrueba(jwt, persona);
  }
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
  const hoja = page.getByRole('dialog', { name: /^Historial/ });
  await expect(hoja).toBeVisible();
  await expect(hoja.getByText('Pedido creado').first()).toBeVisible();
});
