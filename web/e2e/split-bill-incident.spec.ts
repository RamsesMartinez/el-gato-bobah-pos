import { test, expect, type Page, type APIRequestContext, type Locator } from '@playwright/test';
import { API, EMPRESA, PASSWORD, USUARIO, tokenDeRequest } from './ambiente';

// REGRESIÓN DEL INCIDENTE DEL 2026-10-04: una mesa de tres que quiso pagar cada quien lo suyo.
//
// Ese día el POS no dividía por productos y la operadora lo improvisó quitando renglones con un
// motivo falso y recapturándolos: 18 cancelaciones que no ocurrieron, el inventario descontado dos
// veces y un pedido que no se pudo cerrar, que bloqueó el corte. Aquí la misma mesa se resuelve con
// «Dividir → Por productos» y tiene que terminar con el pedido saldado, sin cancelar nada, y en no
// más de 20 toques desde la hoja de cobro (SC-001).
//
// El pedido que crea lo deja entregado y cobrado: el ambiente se comparte con una persona.

const A = ['Coca Cola 355ml', 'Chocolate Licuados'];
const B = ['Mini Pizza - Pepperoni'];
const C = ['Chai Miel', 'Kit Kat'];

let taps = 0;
async function tap(target: Locator) {
  taps += 1;
  await target.click();
}

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

async function agregarProducto(page: Page, nombre: string) {
  const buscar = page.getByPlaceholder('Buscar producto…');
  await buscar.fill(nombre);
  await page.getByText(nombre, { exact: true }).first().click();
  const confirmar = page.getByRole('button', { name: /^(Agregar|Confirmar)/ });
  if (await confirmar.isVisible().catch(() => false)) await confirmar.click();
  await buscar.fill('');
}

type Abierto = { id: number; number: number; outstanding: string; total: string };

async function abiertos(request: APIRequestContext, jwt: string): Promise<Abierto[]> {
  const r = await request.get(`${API}/orders/open`, { headers: { Authorization: `Bearer ${jwt}` } });
  return ((await r.json()).items ?? []) as Abierto[];
}

test('la mesa del incidente se divide por productos sin cancelar nada', async ({ page, request }) => {
  const jwt = await tokenDeRequest(request);
  await entrar(page);

  // La mesa pide todo en una sola cuenta, como ese día.
  for (const p of [...A, ...B, ...C]) await agregarProducto(page, p);
  const antes = new Set((await abiertos(request, jwt)).map((o) => o.id));
  const pildora = page.getByRole('button', { name: /art ·|Ver pedido/ }).first();
  if (await pildora.isVisible().catch(() => false)) await pildora.click();
  await page.getByRole('button', { name: 'Enviar a cocina' }).click();
  let pedido: Abierto | undefined;
  await expect.poll(async () => {
    pedido = (await abiertos(request, jwt)).find((o) => !antes.has(o.id));
    return pedido?.id ?? 0;
  }, { timeout: 20_000 }).toBeGreaterThan(0);
  const nuevo = page.getByRole('button', { name: 'Nuevo pedido' });
  if (await nuevo.isVisible().catch(() => false)) await nuevo.click();

  // Desde «Pedidos por cobrar» del POS: en el tablero, un pedido con algo por entregar ofrece
  // entregar, no cobrar (se cobra cuando está listo).
  taps = 0;
  await tap(page.getByRole('button', { name: /^\$[\d,.]+ \(\d+\)$/ }));
  const lista = page.getByRole('dialog').last();
  const renglon = lista.locator('div').filter({ has: page.getByText(new RegExp(`^#${pedido!.number} · `)) })
    .filter({ has: page.getByRole('button', { name: /^Cobrar/ }) }).last();
  await tap(renglon.getByRole('button', { name: /^Cobrar/ }));
  const hoja = page.getByRole('dialog').last();
  await tap(hoja.getByRole('button', { name: /Dividir/ }));
  await expect(hoja.getByRole('button', { name: 'Por productos', pressed: true })).toBeVisible();

  const cobrarA = async (productos: string[]) => {
    for (const p of productos) await tap(hoja.getByRole('button', { name: new RegExp(`^${p}`) }));
    await tap(hoja.getByRole('button', { name: 'Efectivo' }));
    const cobrar = hoja.getByRole('button', { name: /^Cobrar \$/ });
    await expect(cobrar).toBeEnabled({ timeout: 10_000 });
    await tap(cobrar);
  };
  await cobrarA(A);
  await expect(hoja.getByRole('button', { name: 'Pago 1' })).toBeVisible();
  await cobrarA(B);
  await expect(hoja.getByRole('button', { name: 'Pago 2' })).toBeVisible();
  // La tercera persona paga lo que falta, sin elegir uno por uno.
  await tap(hoja.getByRole('button', { name: 'Todo lo que falta' }));
  await tap(hoja.getByRole('button', { name: 'Efectivo' }));
  const resto = hoja.getByRole('button', { name: /^Cobrar \$/ });
  await expect(resto).toBeEnabled({ timeout: 10_000 });
  await tap(resto);
  await expect(page.getByText('Cobrado').first()).toBeVisible();
  expect(taps, 'la mesa de tres no puede costar más de 20 toques desde la tarjeta').toBeLessThanOrEqual(20);

  // Lo que dice el servidor: tres pagos que suman el total, y nada cancelado.
  const r = await request.get(`${API}/orders/${pedido!.id}`, { headers: { Authorization: `Bearer ${jwt}` } });
  const v = await r.json() as { total: string; outstanding: string; payments: Array<{ amount: string; voided: boolean }>;
    lines: Array<{ cancelled: boolean }> };
  expect(v.outstanding).toMatch(/^0(\.00)?$/);
  expect(v.payments.filter((p) => !p.voided)).toHaveLength(3);
  const suma = v.payments.reduce((n, p) => n + Number(p.amount), 0);
  expect(Math.round(suma * 100)).toBe(Math.round(Number(v.total) * 100));
  expect(v.lines.filter((l) => l.cancelled)).toHaveLength(0);

  // Se entrega para no dejarlo abierto en el ambiente compartido.
  await request.post(`${API}/orders/${pedido!.id}/deliver`, { headers: { Authorization: `Bearer ${jwt}` }, data: {} });
});
