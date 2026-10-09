import { test, expect, type Locator } from '@playwright/test';
import { API, pedidosEnCurso, tokenDeRequest } from './ambiente';
import { abrirTicket, botonCobrar, entrar, ponerUnProducto } from './pos';

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

type Abierto = { id: number; number: number };

test('la mesa del incidente se divide por productos sin cancelar nada', async ({ page, request }) => {
  const jwt = await tokenDeRequest(request);
  await entrar(page);

  // La mesa pide todo en una sola cuenta, como ese día.
  for (const p of [...A, ...B, ...C]) await ponerUnProducto(page, p);
  const antes = new Set((await pedidosEnCurso(jwt)).map((o) => o.id));

  // UNA SOLA PUERTA (spec 030): «Enviar y cobrar» desde el ticket manda a cocina y abre la hoja con
  // los tres modos. Ese día «Por productos» no salía desde aquí.
  await abrirTicket(page);
  taps = 0;
  await tap(botonCobrar(page));
  let pedido: Abierto | undefined;
  await expect.poll(async () => {
    pedido = (await pedidosEnCurso(jwt)).find((o) => !antes.has(o.id));
    return pedido?.id ?? 0;
  }, { timeout: 20_000 }).toBeGreaterThan(0);
  const hoja = page.getByRole('dialog').last();
  await tap(hoja.getByRole('button', { name: /Dividir/ }));
  // El primer modo es «Por productos»; si la hoja abrió antes de traer los renglones se elige.
  const porProductos = hoja.getByRole('button', { name: 'Por productos' });
  if (await porProductos.getAttribute('aria-pressed') !== 'true') await tap(porProductos);

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
  expect(taps, 'la mesa de tres no puede costar más de 20 toques desde el ticket').toBeLessThanOrEqual(20);

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
