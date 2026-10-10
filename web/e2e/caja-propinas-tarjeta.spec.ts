import { test, expect, type Page } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { API, tokenDeApi } from './ambiente';
import { iniciarSesion } from './sesion';
import { pedidoPorApi, entregarPorApi } from './pos';

// CAJA, PROPINAS Y TARJETA (spec 032), contra el ambiente desplegado.
//
// Lo que aquí se ve y en los tests de integración no: que la hoja de reparto, la de cobro y la de
// devolución digan lo mismo que el servidor. Los casos NO cierran ni abren la caja: el ambiente lo
// comparte una persona, y un cierre a media corrida le deja un corte que no hizo. El cierre y la
// apertura a ciegas se probaron a mano el 2026-10-10.
//
// Cada caso deja el ambiente como lo encontró: la propina que cobra la reparte completa, la
// terminal que crea la archiva y la preferencia que mueve la devuelve.

const cab = (jwt: string) => ({ Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' });

async function get<T>(jwt: string, ruta: string): Promise<T> {
  const r = await fetch(`${API}${ruta}`, { headers: cab(jwt) });
  if (!r.ok) throw new Error(`GET ${ruta}: ${r.status}`);
  return r.json() as Promise<T>;
}

async function enviar(jwt: string, metodo: string, ruta: string, cuerpo: unknown) {
  return fetch(`${API}${ruta}`, { method: metodo, headers: cab(jwt), body: JSON.stringify(cuerpo) });
}

type Metodo = { id: number; name: string; kind: string; deliveryPlatformId: number | null };
async function metodo(jwt: string, nombre: string): Promise<Metodo> {
  const r = await get<{ items?: Metodo[] } | Metodo[]>(jwt, '/payment-methods');
  const m = (Array.isArray(r) ? r : r.items ?? []).find((x) => x.name === nombre);
  if (!m) throw new Error(`el ambiente no tiene el método «${nombre}»`);
  return m;
}

type Turno = {
  id: number;
  tipsPending: { total: string };
  drawer: { expected: string };
  breakdown: { ingresos: Array<{ method: string; items: Array<{ concept: string; amount: string }> }> };
};

// turnoDeLaPrincipal: el turno abierto de la caja que vende, o null si no hay.
async function turnoDeLaPrincipal(jwt: string): Promise<{ registerId: number; turno: Turno } | null> {
  const cajas = await get<{ items: Array<{ id: number; isPrimary: boolean; openSessionId: number | null }> }>(jwt, '/cash-registers');
  const p = cajas.items.find((c) => c.isPrimary && c.openSessionId !== null);
  if (!p) return null;
  return { registerId: p.id, turno: await get<Turno>(jwt, `/cash-sessions/current?registerId=${p.id}`) };
}

const n = (s: string | undefined) => Number(s ?? 0);
const concepto = (t: Turno, metodo: string, c: string) =>
  n(t.breakdown.ingresos.find((i) => i.method === metodo)?.items.find((i) => i.concept === c)?.amount);

async function entrar(page: Page, ruta: string) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  await expect(page.getByRole('button', { name: 'Salir' })).toBeVisible({ timeout: 30_000 });
  await page.goto(ruta);
}

test('la propina no es venta, se reparte en pesos enteros y sale del cajón una sola vez', async ({ page }) => {
  const jwt = await tokenDeApi();
  const antes = await turnoDeLaPrincipal(jwt);
  test.skip(antes === null, 'no hay caja abierta en el ambiente: no se abre la de alguien más');
  const { registerId } = antes!;

  const PROPINA = 3;
  const pedido = await pedidoPorApi(jwt);
  const efectivo = await metodo(jwt, 'Efectivo');
  const r = await enviar(jwt, 'POST', `/orders/${pedido.id}/pay`,
    { methodId: efectivo.id, amount: Number(pedido.total), tip: PROPINA, clientUuid: randomUUID() });
  expect(r.ok, `cobrar con propina: ${r.status}`).toBe(true);
  await entregarPorApi(jwt, pedido.id);

  const cobrado = (await turnoDeLaPrincipal(jwt))!.turno;
  // Si la propina se contara como venta, «Ventas» subiría por el total más la propina.
  expect(concepto(cobrado, 'Efectivo', 'Ventas') - concepto(antes!.turno, 'Efectivo', 'Ventas'),
    'la propina entró como venta').toBeCloseTo(Number(pedido.total), 2);
  expect(n(cobrado.tipsPending.total) - n(antes!.turno.tipsPending.total)).toBeCloseTo(PROPINA, 2);

  await entrar(page, '/caja');
  await page.getByRole('button', { name: 'Entregar propina' }).first().click();
  const hoja = page.getByRole('dialog').last();
  await hoja.getByRole('button', { name: 'Usuario 5', exact: true }).click();
  await hoja.getByRole('button', { name: 'Ajustar montos' }).click();
  const monto = hoja.getByLabel('Monto para Usuario 5');
  const entregar = hoja.getByRole('button', { name: /^Entregar a 1 persona/ });

  // D-B: los centavos no se reparten.
  await monto.fill('1.50');
  await expect(hoja.getByText('La propina se entrega en pesos enteros')).toBeVisible();
  await expect(entregar).toBeDisabled();
  // EB-06: más que lo pendiente deja al cajón pagando con dinero del negocio.
  await monto.fill(String(Math.floor(n(cobrado.tipsPending.total)) + 100));
  await expect(hoja.getByText('No hay tanta propina por entregar')).toBeVisible();
  await expect(entregar).toBeDisabled();

  await monto.fill(String(PROPINA));
  await entregar.click();
  await expect(page.getByRole('dialog')).toHaveCount(0, { timeout: 15_000 });

  const despues = (await turnoDeLaPrincipal(jwt))!.turno;
  expect(n(despues.tipsPending.total), 'el pendiente no bajó por lo entregado').toBeCloseTo(n(antes!.turno.tipsPending.total), 2);
  // EB-03: la propina en efectivo entra una vez al cobrar y sale una vez al entregarla.
  expect(n(despues.drawer.expected) - n(antes!.turno.drawer.expected),
    'el cajón no quedó con exactamente lo vendido').toBeCloseTo(Number(pedido.total), 2);
  expect(concepto(despues, 'Efectivo', 'Ventas'), 'entregar la propina movió las ventas').toBeCloseTo(concepto(cobrado, 'Efectivo', 'Ventas'), 2);

  // Las mismas reglas en el servidor, no solo en la hoja.
  const centavos = await enviar(jwt, 'POST', '/cash-sessions/tips/payouts',
    { registerId, mode: 'ajustado', recipients: [{ userId: 5, amount: 1.5 }] });
  expect(centavos.status).toBe(400);
  const repetida = await enviar(jwt, 'POST', '/cash-sessions/tips/payouts',
    { registerId, mode: 'parejo', recipients: [{ userId: 5 }, { userId: 5 }] });
  expect(repetida.status).toBe(400);
});

test('la devolución de un cobro con tarjeta no se registra sin el folio de la terminal', async ({ page }) => {
  const jwt = await tokenDeApi();
  test.skip((await turnoDeLaPrincipal(jwt)) === null, 'no hay caja abierta en el ambiente');
  const pedido = await pedidoPorApi(jwt);
  const debito = await metodo(jwt, 'Tarjeta débito');
  const r = await enviar(jwt, 'POST', `/orders/${pedido.id}/pay`,
    { methodId: debito.id, amount: Number(pedido.total), tip: 0, clientUuid: randomUUID() });
  expect(r.ok, `cobrar con tarjeta: ${r.status}`).toBe(true);
  await entregarPorApi(jwt, pedido.id);

  // EB-34/EB-36: sin folio, o con puros espacios, el servidor no la registra.
  for (const cardFolio of [undefined, '   ']) {
    const sin = await enviar(jwt, 'POST', `/orders/${pedido.id}/refund`, { amount: 1, reason: 'Producto en mal estado', cardFolio });
    expect(sin.status, `folio ${JSON.stringify(cardFolio)}`).toBe(422);
  }

  await entrar(page, '/pedidos');
  const tarjeta = page.getByText(pedido.folioName, { exact: true }).first()
    .locator('xpath=ancestor::*[.//button[normalize-space()="Devolver"]][1]');
  await tarjeta.getByRole('button', { name: 'Devolver' }).click();
  const hoja = page.getByRole('dialog').last();
  await expect(hoja.getByText(/Devuélvelo en la terminal/)).toBeVisible();
  const devolver = hoja.getByRole('button', { name: /^Devolver \$/ });
  await expect(devolver).toBeDisabled();
  await hoja.getByLabel('Folio de la terminal').fill('   ');
  await expect(devolver).toBeDisabled();
  await hoja.getByLabel('Folio de la terminal').fill('E2E-1');
  await expect(devolver).toBeEnabled();
  // No se devuelve: el pedido queda cobrado y entregado, como cualquier otro de la suite.
  await page.keyboard.press('Escape');
});

test('la terminal que el usuario usó la última vez llega puesta al cobrar con tarjeta', async ({ page }) => {
  // DEFECTO ABIERTO (2026-10-10): `posApi.defaultTerminal` lee `/me/preferences/card_terminal` como
  // si fuera el id, y el servidor responde `{ "value": 3 }`. Con una sola terminal no se nota —la
  // hoja cae a «la única activa»—, pero en cuanto el negocio da de alta una segunda, cada cobro con
  // tarjeta pide elegirla otra vez y «Cobrar» queda apagado hasta hacerlo. Quita esta línea al
  // arreglarlo: el caso tiene que pasar.
  test.fail(true, 'la hoja de cobro no entiende la forma de /me/preferences');

  const jwt = await tokenDeApi();
  test.skip((await turnoDeLaPrincipal(jwt)) === null, 'no hay caja abierta en el ambiente');
  type Terminal = { id: number; branchId: number; name: string; archived: boolean };
  const terminales = (await get<{ items: Terminal[] }>(jwt, '/card-terminals')).items.filter((t) => !t.archived);
  test.skip(terminales.length === 0, 'el ambiente no tiene terminales');
  const previa = (await get<{ value: number | null }>(jwt, '/me/preferences/card_terminal')).value;

  const creada = await enviar(jwt, 'POST', '/card-terminals', { branchId: terminales[0].branchId, name: `E2E ${Date.now() % 100000}` });
  expect(creada.ok, `alta de terminal: ${creada.status}`).toBe(true);
  const nueva = (await creada.json()) as Terminal;
  try {
    await enviar(jwt, 'PUT', '/me/preferences/card_terminal', nueva.id);
    const pedido = await pedidoPorApi(jwt);
    await entrar(page, `/pos?pedido=${pedido.id}`);
    const cobrar = page.getByRole('button', { name: /^(Enviar y cobrar|Cobrar)( \$[\d,.]+)?$/ }).filter({ visible: true }).first();
    await expect(cobrar).toBeVisible({ timeout: 30_000 });
    await cobrar.click();
    const hoja = page.getByRole('dialog').last();
    await hoja.getByRole('button', { name: 'Tarjeta débito', exact: true }).click();
    await expect(hoja.getByRole('button', { name: nueva.name })).toBeVisible();
    await expect(hoja.getByRole('button', { name: /^Cobrar \$/ })).toBeEnabled();
    await page.keyboard.press('Escape');
  } finally {
    await enviar(jwt, 'PATCH', `/card-terminals/${nueva.id}`, { archived: true });
    await enviar(jwt, 'PUT', '/me/preferences/card_terminal', previa);
  }
});

test('los correos del resumen diario se validan en el servidor', async () => {
  const jwt = await tokenDeApi();
  const antes = await get<{ emails: string[] }>(jwt, '/settings/daily-summary-emails');
  const casos: Array<[string, string[]]> = [
    ['mal formado', ['no-es-correo']],
    ['repetido sin distinguir mayúsculas', ['e2e@example.com', 'E2E@example.com']],
    ['más de 10', Array.from({ length: 11 }, (_, i) => `e2e${i}@example.com`)],
  ];
  for (const [nombre, emails] of casos) {
    const r = await enviar(jwt, 'PUT', '/settings/daily-summary-emails', { emails });
    expect(r.status, nombre).toBe(400);
  }
  expect((await get<{ emails: string[] }>(jwt, '/settings/daily-summary-emails')).emails, 'un rechazo cambió la lista').toEqual(antes.emails);
});

test('un concepto de salida escrito con otras mayúsculas o espacios es el mismo', async () => {
  const jwt = await tokenDeApi();
  type Concepto = { id: number; name: string };
  const lista = (await get<{ items: Concepto[] }>(jwt, '/cash-concepts')).items;
  test.skip(lista.length === 0, 'el ambiente no tiene conceptos');
  const r = await enviar(jwt, 'POST', '/cash-concepts', { name: `  ${lista[0].name.toUpperCase()} ` });
  expect(r.ok, `alta: ${r.status}`).toBe(true);
  expect(((await r.json()) as Concepto).id, 'nació un duplicado').toBe(lista[0].id);
  expect((await get<{ items: Concepto[] }>(jwt, '/cash-concepts')).items).toHaveLength(lista.length);
});
