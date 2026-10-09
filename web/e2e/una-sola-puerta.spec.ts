import { test, expect, type Browser, type Page } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { API, cuentasVivas, pedidosEnCurso, tokenDeApi, type CuentaViva } from './ambiente';
import {
  abrirTicket, botonCobrar, botonEnviar, crearCuentaPorApi, entrar, entregarPorApi, fichas,
  nombreDeLaCuentaActiva, pagarPorApi, pedido, pedidoPorApi, ponerUnProducto, productoPorNombre,
} from './pos';

// UNA SOLA PUERTA PARA COBRAR, CASO POR CASO (spec 030; lienzo V2-9).
//
// Un test por caso del lienzo que se ve en la pantalla —el número va en el título— y por historia
// del spec. Dos contextos de navegador son dos tabletas: comparten el servidor y nada más, que es
// justo lo que la feature promete. Los casos que solo se prueban en el servidor o en vitest están en
// el mapa de tasks.md.
//
// Todo lo que se crea aquí lo descarta, cobra y entrega la limpieza de la suite al terminar.

async function otraTableta(browser: Browser): Promise<Page> {
  const ctx = await browser.newContext({ viewport: { width: 1024, height: 600 } });
  const p = await ctx.newPage();
  await entrar(p);
  return p;
}

const ficha = (page: Page, nombre: string, estado?: string) =>
  page.getByRole('button', { name: new RegExp(`^${nombre} · ${estado ?? ''}`) });

// miCuenta: la cuenta en captura de ESTA tableta en el servidor, por el nombre que dice su ficha.
// El nombre es único entre las cuentas vivas (spec 030, US2), así que no hay dos que coincidan.
async function miCuenta(page: Page, jwt: string): Promise<CuentaViva> {
  const nombre = await nombreDeLaCuentaActiva(page);
  const c = (await cuentasVivas(jwt)).find((x) => x.kind === 'draft' && x.folioName === nombre);
  if (!c) throw new Error(`la cuenta «${nombre}» no está en el servidor`);
  return c;
}

async function abrirPedido(page: Page, id: number) {
  await page.goto(`/pos?pedido=${id}`);
  await abrirTicket(page);
}

test.describe('la fila de cuentas (US1)', () => {
  test('caso 1 · historia 1 · una cuenta capturándose en otra tableta aparece en la fila', async ({ page, browser }) => {
    await entrar(page);
    await ponerUnProducto(page);
    const nombre = (await miCuenta(page, await tokenDeApi())).folioName ?? '';
    expect(nombre).not.toBe('');
    const b = await otraTableta(browser);
    await expect(ficha(b, nombre, 'Capturando').or(b.getByRole('button', { name: /Ver todas las cuentas/ })))
      .toBeVisible({ timeout: 30_000 });
    await b.getByRole('button', { name: /Ver todas las cuentas/ }).click();
    await expect(b.getByRole('dialog').last().getByRole('button', { name: new RegExp(`^${nombre} · Capturando`) }))
      .toBeVisible();
    await b.context().close();
  });

  test('caso 2 · pagado en cocina sigue en la fila y recibe productos', async ({ page }) => {
    const jwt = await tokenDeApi();
    const o = await pedidoPorApi(jwt);
    await pagarPorApi(jwt, o.id);
    await entrar(page);
    await abrirPedido(page, o.id);
    await expect(ficha(page, o.folioName, 'Pagada · en cocina')).toBeVisible({ timeout: 30_000 });
    await ponerUnProducto(page, 'Coca Cola 355ml');
    await abrirTicket(page);
    await expect(page.getByRole('heading', { name: /Nuevo · aún no va a cocina/ })).toBeVisible();
  });

  test('caso 3 · una deuda de días anteriores está en «De días anteriores»', async ({ page }) => {
    const jwt = await tokenDeApi();
    const r = await fetch(`${API}/pos/accounts?olderDebts=true`, { headers: { Authorization: `Bearer ${jwt}` } });
    const vieja = ((await r.json()).items ?? []).find((c: { group: string }) => c.group === 'previous_days');
    test.skip(!vieja, 'el ambiente no tiene deudas de días anteriores y no se pueden fabricar: la fecha la pone el reloj');
    await entrar(page);
    await page.getByRole('button', { name: /^Ver todas las cuentas/ }).click();
    const hoja = page.getByRole('dialog').last();
    await expect(hoja.getByRole('heading', { name: /De días anteriores/ })).toBeVisible({ timeout: 20_000 });
  });

  test('caso 4 · entregada que debe se ve en rojo; pagada y entregada sale de la fila', async ({ page }) => {
    const jwt = await tokenDeApi();
    const o = await pedidoPorApi(jwt);
    await entregarPorApi(jwt, o.id);
    await entrar(page);
    await abrirPedido(page, o.id);
    await expect(ficha(page, o.folioName, 'Entregada · debe')).toBeVisible({ timeout: 30_000 });
    await pagarPorApi(jwt, o.id);
    await page.getByRole('button', { name: 'Cuenta nueva', exact: true }).click();
    await page.reload();
    await expect(fichas(page).filter({ hasText: o.folioName })).toHaveCount(0, { timeout: 30_000 });
  });

  test('caso 25 · el tablero lleva a la cuenta con «Abrir cuenta»', async ({ page }) => {
    const jwt = await tokenDeApi();
    const o = await pedidoPorApi(jwt);
    await entrar(page);
    await page.goto('/pedidos');
    const tarjeta = page.locator('[data-order-card]').filter({ hasText: o.folioName }).first();
    await tarjeta.getByRole('button', { name: 'Abrir cuenta' }).click();
    await expect(page).toHaveURL(/\/pos/);
    await abrirTicket(page);
    await expect(page.getByText(`#${o.number}`).first()).toBeVisible({ timeout: 30_000 });
  });

  test('caso 29 · la fila no se desborda (la medición vive en cabe-en-la-tableta.spec.ts)', async ({ page }) => {
    await entrar(page);
    const desborda = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
    expect(desborda).toBe(false);
  });
});

test.describe('la cuenta existe desde el primer producto (US2)', () => {
  test('caso 14 · historia 2 · recargar o apagar la tableta no pierde lo capturado', async ({ page, browser }) => {
    await entrar(page);
    await ponerUnProducto(page);
    await ponerUnProducto(page, 'Coca Cola 355ml');
    const jwt = await tokenDeApi();
    const cuenta = await miCuenta(page, jwt);

    await page.reload();
    await abrirTicket(page);
    await expect(page.getByText('Coca Cola 355ml').first()).toBeVisible({ timeout: 30_000 });

    // La tableta se apaga: se cierra su contexto. La otra la ve con sus dos productos.
    await page.context().close();
    const b = await otraTableta(browser);
    await b.goto(`/pos?cuenta=${cuenta.draftId}`);
    await abrirTicket(b);
    await expect(b.getByText('Dedos de Queso Pza').first()).toBeVisible({ timeout: 30_000 });
    await expect(b.getByText('Coca Cola 355ml').first()).toBeVisible();
    await b.context().close();
  });

  test('caso 18 · una cuenta guardada por la versión anterior sube al servidor', async ({ page }) => {
    const jwt = await tokenDeApi();
    const productId = await productoPorNombre(jwt, 'Dedos de Queso Pza');
    const id = randomUUID();
    await page.addInitScript(({ id, productId }) => {
      localStorage.setItem('egb:ticket:v2', JSON.stringify({
        state: {
          tabs: [{ id, num: 1, folioName: 'Persa', lines: [{ lineId: 'x', productId, name: 'Dedos', unitPrice: 12, qty: 1, modifiers: [] }],
            serviceType: 'mostrador', customerName: '', platformId: null }],
          activeId: id, seq: 2,
        },
        version: 0,
      }));
    }, { id, productId });
    await entrar(page);
    await expect.poll(async () => (await cuentasVivas(jwt)).some((c) => c.draftId === id), { timeout: 20_000 }).toBe(true);
  });

  test('caso 23 · el nombre de la cuenta es el del pedido al mandarla', async ({ page }) => {
    await entrar(page);
    await ponerUnProducto(page);
    const jwt = await tokenDeApi();
    const antes = new Set((await pedidosEnCurso(jwt)).map((o) => o.id));
    const nombre = (await miCuenta(page, jwt)).folioName;
    await abrirTicket(page);
    await botonEnviar(page).click();
    await expect.poll(async () => (await pedidosEnCurso(jwt)).find((o) => !antes.has(o.id))?.folioName ?? '',
      { timeout: 20_000 }).toBe(nombre);
  });
});

test.describe('agregar después de cocina (US3)', () => {
  test('caso 5 · mandar a cocina deja la cuenta en pantalla y en la fila', async ({ page }) => {
    await entrar(page);
    await ponerUnProducto(page);
    await abrirTicket(page);
    await botonEnviar(page).click();
    await expect(page.getByRole('heading', { name: /En cocina/ })).toBeVisible({ timeout: 20_000 });
    await expect(fichas(page).filter({ hasText: 'En cocina' }).first()).toBeVisible();
  });

  test('caso 6 · historia 3 · agregar después manda solo lo nuevo', async ({ page }) => {
    const jwt = await tokenDeApi();
    const o = await pedidoPorApi(jwt);
    const antes = (await pedido(jwt, o.id)).lines?.length ?? 0;
    await entrar(page);
    await abrirPedido(page, o.id);
    await ponerUnProducto(page, 'Coca Cola 355ml');
    await abrirTicket(page);
    await expect(page.getByRole('heading', { name: /Nuevo · aún no va a cocina/ })).toBeVisible();
    // Cocina no lo ve todavía.
    expect((await pedido(jwt, o.id)).lines?.length ?? 0).toBe(antes);
    await page.getByRole('button', { name: 'Enviar 1 a cocina' }).click();
    await expect.poll(async () => (await pedido(jwt, o.id)).lines?.length ?? 0, { timeout: 20_000 }).toBe(antes + 1);
  });

  test('caso 7 · agregar a una entregada que debe la regresa a cocina', async ({ page }) => {
    const jwt = await tokenDeApi();
    const o = await pedidoPorApi(jwt);
    await entregarPorApi(jwt, o.id);
    await entrar(page);
    await abrirPedido(page, o.id);
    await ponerUnProducto(page, 'Coca Cola 355ml');
    await abrirTicket(page);
    await page.getByRole('button', { name: 'Enviar 1 a cocina' }).click();
    await expect.poll(async () => (await pedido(jwt, o.id)).status, { timeout: 20_000 }).toBe('abierta');
  });

  test('caso 8 · historia 6 · quitar 1 de 2 en cocina avisa y baja el total', async ({ page }) => {
    const jwt = await tokenDeApi();
    const o = await pedidoPorApi(jwt, 'Dedos de Queso Pza', '2');
    const total = Number(o.total);
    await entrar(page);
    await abrirPedido(page, o.id);
    await page.getByRole('button', { name: /^Opciones de / }).first().click();
    await page.getByRole('menuitem', { name: /Quitar/ }).click();
    const dialogo = page.getByRole('dialog').last();
    await expect(dialogo.getByText(/1 de 2/)).toBeVisible();
    // Los motivos son un grupo de opciones, sin ninguna preseleccionada (historia 6).
    await dialogo.getByRole('radio', { name: 'Ya no lo quiere' }).click();
    await dialogo.getByRole('button', { name: 'Quitar del pedido' }).click();
    await expect.poll(async () => Number((await pedido(jwt, o.id)).total), { timeout: 20_000 }).toBeLessThan(total);
  });

  test('caso 19 · un pedido de plataforma no recibe productos ni se divide', async ({ page }) => {
    await entrar(page);
    const uber = page.getByRole('button', { name: /Uber Eats/ });
    test.skip(!(await uber.isVisible().catch(() => false)), 'este negocio no tiene plataformas configuradas');
    await uber.click();
    await ponerUnProducto(page);
    await abrirTicket(page);
    await botonCobrar(page).click();
    const sinFolio = page.getByRole('button', { name: /sin folio/i });
    if (await sinFolio.isVisible().catch(() => false)) await sinFolio.click();
    const hoja = page.getByRole('dialog').last();
    await expect(hoja.getByText(/Falta \$/)).toBeVisible({ timeout: 30_000 });
    await expect(hoja.getByRole('button', { name: 'Dividir' })).toHaveCount(0);
    await page.keyboard.press('Escape');
    await ponerUnProducto(page, 'Coca Cola 355ml').catch(() => {});
    await expect(page.getByText('A un pedido de plataforma no se le agregan productos.')).toBeVisible();
  });
});

test.describe('cobrar siempre ofrece los tres modos (US4)', () => {
  test('caso 26 · historia 4 · «Por productos» sale desde el ticket', async ({ page }) => {
    await entrar(page);
    await ponerUnProducto(page);
    await ponerUnProducto(page, 'Coca Cola 355ml');
    await abrirTicket(page);
    await botonCobrar(page).click();
    const hoja = page.getByRole('dialog').last();
    await hoja.getByRole('button', { name: 'Dividir' }).click();
    for (const modo of ['Por productos', 'Entre personas', 'Por monto']) {
      await expect(hoja.getByRole('button', { name: modo })).toBeVisible();
    }
  });

  test('caso 11 · cerrar la hoja de cobro no cancela nada', async ({ page }) => {
    await entrar(page);
    await ponerUnProducto(page);
    await abrirTicket(page);
    await botonCobrar(page).click();
    await expect(page.getByRole('dialog').last().getByText(/Falta \$/)).toBeVisible({ timeout: 30_000 });
    await page.keyboard.press('Escape');
    await expect(fichas(page).filter({ hasText: 'En cocina' }).first()).toBeVisible({ timeout: 20_000 });
  });

  test('caso 15 · la red cae al mandar y el reintento no crea dos pedidos', async ({ page }) => {
    const jwt = await tokenDeApi();
    const antes = new Set((await pedidosEnCurso(jwt)).map((o) => o.id));
    await entrar(page);
    await ponerUnProducto(page);
    await abrirTicket(page);
    // El servidor procesa el envío y la respuesta se pierde.
    let cortadas = 0;
    await page.route('**/pos/drafts/*/send', async (route) => {
      if (cortadas++ === 0) { await route.fetch(); await route.abort('connectionreset'); return; }
      await route.continue();
    });
    await botonEnviar(page).click();
    await page.waitForTimeout(1_500);
    if (await botonEnviar(page).isEnabled().catch(() => false)) await botonEnviar(page).click();
    await page.waitForTimeout(2_000);
    const nuevos = (await pedidosEnCurso(jwt)).filter((o) => !antes.has(o.id));
    expect(nuevos, 'el reintento creó otro pedido: cocina prepara dos veces lo mismo').toHaveLength(1);
  });
});

test.describe('nada se cierra ni se pierde por accidente (US5)', () => {
  test('historia 5 · vacía se descarta sin preguntar; con productos pregunta y el nombre vuelve a la bolsa', async ({ page }) => {
    await entrar(page);
    await ponerUnProducto(page);
    const jwt = await tokenDeApi();
    const nombre = (await miCuenta(page, jwt)).folioName!;
    await abrirTicket(page);
    await page.getByRole('button', { name: 'Más opciones de la cuenta' }).click();
    await page.getByRole('menuitem', { name: /Descartar cuenta/ }).click();
    const hoja = page.getByRole('dialog').last();
    await expect(hoja.getByText(`¿Descartar la cuenta de ${nombre}?`)).toBeVisible();
    await hoja.getByRole('button', { name: 'Descartar', exact: true }).click();
    await expect.poll(async () => (await cuentasVivas(jwt)).some((c) => c.folioName === nombre && c.kind === 'draft'),
      { timeout: 20_000 }).toBe(false);
    const nombres = await (await fetch(`${API}/pos/folio-names`, { headers: { Authorization: `Bearer ${jwt}` } })).json();
    // La bolsa guarda animales: una cuenta que nació «Tonkinés 2» —el turno ya había cantado la lista
    // entera— devuelve «Tonkinés».
    const animal = nombre.replace(/ \d+$/, '');
    expect(nombres.items, 'el nombre de la cuenta descartada no volvió a la bolsa').toContain(animal);
  });

  test('caso 10 · ningún diálogo del sistema al cerrar o cancelar', async ({ page }) => {
    let dialogos = 0;
    page.on('dialog', async (d) => { dialogos++; await d.dismiss(); });
    await entrar(page);
    await ponerUnProducto(page);
    await abrirTicket(page);
    await page.getByRole('button', { name: 'Más opciones de la cuenta' }).click();
    await page.getByRole('menuitem', { name: /Descartar cuenta/ }).click();
    await page.getByRole('dialog').last().getByRole('button', { name: 'Seguir capturando' }).click();
    expect(dialogos, 'apareció un confirm() del navegador').toBe(0);
  });
});

test.describe('red, recarga y otra tableta (US7)', () => {
  test('caso 16 · historia 7 · otra tableta cobró la cuenta y ésta lo dice', async ({ page }) => {
    const jwt = await tokenDeApi();
    const o = await pedidoPorApi(jwt);
    await entrar(page);
    await abrirPedido(page, o.id);
    await expect(page.getByText(`#${o.number}`).first()).toBeVisible({ timeout: 30_000 });
    await pagarPorApi(jwt, o.id);
    await expect(page.getByText(`${o.folioName} se cobró en otra tableta`)).toBeVisible({ timeout: 40_000 });
  });

  // FR-017: el aviso es para la tableta que NO cobró. La que cobra ve su propio «Cobrado» y nada
  // más; si se avisara a sí misma, quien cobra creería que otra persona le ganó la cuenta.
  test('caso 16 bis · la tableta que cobra no se avisa a sí misma; la otra sí', async ({ page, browser }) => {
    const jwt = await tokenDeApi();
    const o = await pedidoPorApi(jwt);
    const aviso = `${o.folioName} se cobró en otra tableta`;
    await entrar(page);
    await abrirPedido(page, o.id);
    await expect(page.getByText(`#${o.number}`).first()).toBeVisible({ timeout: 30_000 });
    const b = await otraTableta(browser);
    await abrirPedido(b, o.id);
    await expect(b.getByText(`#${o.number}`).first()).toBeVisible({ timeout: 30_000 });

    // El aviso puede ser un toast que dura segundos: se anota si llegó a aparecer, no si sigue ahí.
    await page.evaluate((texto) => {
      const w = window as unknown as { __avisoPropio?: boolean };
      new MutationObserver(() => { if (document.body.innerText.includes(texto)) w.__avisoPropio = true; })
        .observe(document.body, { childList: true, subtree: true, characterData: true });
    }, aviso);

    await botonCobrar(page).click();
    const hoja = page.getByRole('dialog').last();
    await hoja.getByRole('button', { name: 'Efectivo', exact: true }).click();
    await hoja.getByRole('button', { name: /^Cobrar \$/ }).click();
    await expect(page.getByText(/^Cobrado ·/)).toBeVisible({ timeout: 30_000 });

    await expect(b.getByText(aviso)).toBeVisible({ timeout: 40_000 });
    // Margen para que el eco del cobro le llegue también a la que cobró.
    await page.waitForTimeout(5_000);
    const seAviso = await page.evaluate(() => (window as unknown as { __avisoPropio?: boolean }).__avisoPropio === true);
    expect(seAviso, 'la tableta que cobró se avisó a sí misma que «se cobró en otra tableta»').toBe(false);
    await b.context().close();
  });

  test('caso 17 · dos tabletas agregan a la misma cuenta y se suman', async ({ page }) => {
    await entrar(page);
    await ponerUnProducto(page);
    const jwt = await tokenDeApi();
    const cuenta = await miCuenta(page, jwt);
    const productId = await productoPorNombre(jwt, 'Dedos de Queso Pza');
    // La otra tableta agrega el mismo producto.
    const r = await fetch(`${API}/pos/drafts/${cuenta.draftId}/lines`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ opId: randomUUID(), productId, qty: '1', modifiers: [], notes: '' }),
    });
    expect(r.ok, `la otra tableta no pudo agregar: ${r.status}`).toBe(true);
    await page.reload();
    await abrirTicket(page);
    await expect(page.getByText('2', { exact: true }).first()).toBeVisible({ timeout: 30_000 });
  });

  test('caso 17 bis · sin conexión no se agrega en silencio y se dice', async ({ page, context }) => {
    await entrar(page);
    await context.setOffline(true);
    await expect(page.getByRole('alert').filter({ hasText: 'Sin conexión' })).toBeVisible();
    await context.setOffline(false);
  });
});

test.describe('cerrar caja con cuentas vivas (US8)', () => {
  test('caso 21 · caso 12 · historia 8 · el cierre lista las cuentas vivas y «Abrir» lleva a cada una', async ({ page }) => {
    const jwt = await tokenDeApi();
    await crearCuentaPorApi(jwt, await productoPorNombre(jwt, 'Dedos de Queso Pza'));
    await entrar(page);
    await page.goto('/caja');
    const plegada = page.getByRole('button', { name: /Cuentas pendientes \(\d+\)/ });
    test.skip(!(await plegada.isVisible({ timeout: 30_000 }).catch(() => false)), 'no hay una caja abierta en el ambiente');
    await plegada.click();
    await page.getByRole('button', { name: /^Abrir / }).first().click();
    await expect(page).toHaveURL(/\/pos/);
    await abrirTicket(page);
    await expect(botonCobrar(page)).toBeVisible({ timeout: 30_000 });
  });

  // Lo que bloquea el cierre se lee con su monto: «Persa · #12  $12». Sin la cifra, quien cierra no
  // sabe si lo que falta entregar es un refresco o la mesa grande.
  test('el cierre dice el monto de cada pedido en «Falta entregar»', async ({ page }) => {
    const jwt = await tokenDeApi();
    const o = await pedidoPorApi(jwt);
    await entrar(page);
    await page.goto('/caja');
    const caja = page.getByText(/^Falta entregar \d+ pedidos?$/);
    const hay = await caja.waitFor({ timeout: 30_000 }).then(() => true).catch(() => false);
    test.skip(!hay, 'no hay una caja abierta en el ambiente');
    const renglon = page.getByRole('button', { name: `Abrir ${o.folioName}`, exact: true }).locator('..');
    await expect(renglon).toContainText(`${o.folioName} · #${o.number}`);
    const entero = Math.trunc(Number(o.total)).toLocaleString('en-US');
    await expect(renglon).toContainText(new RegExp(`\\$${entero}(\\.\\d{2})?`));
  });

  test('caso 13 · cancelar con algo entregado ofrece quitar lo que falta', async ({ page }) => {
    const jwt = await tokenDeApi();
    const o = await pedidoPorApi(jwt, 'Dedos de Queso Pza', '2');
    const linea = (await pedido(jwt, o.id)).lines![0];
    await fetch(`${API}/orders/${o.id}/lines/${linea.id}/deliver`, {
      method: 'POST', headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ qty: 1 }),
    });
    await entrar(page);
    await page.goto('/pedidos');
    const tarjeta = page.locator('[data-order-card]').filter({ hasText: o.folioName }).first();
    await tarjeta.getByRole('button', { name: 'Más' }).click();
    await expect(page.getByRole('menuitem', { name: /Quitar lo que falta/ })).toBeVisible();
  });
});
