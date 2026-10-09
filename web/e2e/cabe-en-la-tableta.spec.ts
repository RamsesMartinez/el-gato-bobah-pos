import { test, expect, type Page } from '@playwright/test';
import { tokenDeApi } from './ambiente';
import {
  abrirTicket, alto, botonCobrar, botonEnviar, crearCuentaPorApi, cuentaNueva, entrar, fichas, fila,
  ponerUnProducto, productoPorNombre,
} from './pos';

// LO QUE SE AGREGA TIENE QUE CABER EN LA TABLETA (SC-004).
//
// El presupuesto real es 1024×600 y el alto es lo que escasea: cada control que se agrega se lo
// quita a lo que el operador vino a leer. Es un requisito funcional del producto y se olvida solo,
// así que se mide en vez de recordarse. Vitest no puede: las medidas de Chakra son clases CSS que
// jsdom no resuelve. Aquí hay un navegador de verdad.

const panelAbierto = (page: Page) => page.getByRole('button', { name: 'Ocultar pedido' }).isVisible().catch(() => false);

async function altoDeLaHoja(page: Page) {
  return Math.round((await page.locator('[role="dialog"]').last().boundingBox())?.height ?? 0);
}

test('E7 · la hoja de cobro cabe en 600 px, con y sin repartir', async ({ page }) => {
  await entrar(page);
  await ponerUnProducto(page);
  await abrirTicket(page);
  await botonCobrar(page).click();

  // Se espera a los métodos de pago: la hoja crece cuando llegan, y medir antes da un alto que el
  // operador nunca ve.
  await expect(page.getByRole('button', { name: 'Efectivo' })).toBeVisible({ timeout: 30_000 });
  const sinRepartir = await altoDeLaHoja(page);
  expect(sinRepartir, 'la hoja no cabe en la tableta sin repartir').toBeLessThanOrEqual(600);

  await page.getByRole('button', { name: /Dividir/ }).click();
  const repartiendo = await altoDeLaHoja(page);
  expect(repartiendo, 'la hoja no cabe en la tableta al repartir').toBeLessThanOrEqual(600);
  // El repartidor no es gratis, y por eso no vive abierto.
  expect(repartiendo - sinRepartir,
    'el repartidor dejó de costar alto: ¿volvió a estar siempre abierto?').toBeGreaterThan(40);
  console.log(`[e2e] hoja de cobro: ${sinRepartir}px sin repartir, ${repartiendo}px repartiendo, de 600px`);
});

// X7 · LOS CONTROLES DEL RENGLÓN NUEVO MIDEN 44 PX Y LA PAPELERA VA APARTE.
test('X7 · los controles del renglón del ticket miden 44 px y la papelera va aparte', async ({ page }) => {
  await entrar(page);
  await ponerUnProducto(page);
  await abrirTicket(page);
  await expect(page.getByRole('button', { name: 'Quitar' }).first()).toBeVisible({ timeout: 30_000 });

  const menos = page.getByRole('button', { name: 'Uno menos' }).first();
  const mas = page.getByRole('button', { name: 'Uno más' }).first();
  const quitar = page.getByRole('button', { name: 'Quitar' }).first();
  for (const [nombre, boton] of [['−', menos], ['+', mas], ['Quitar', quitar]] as const) {
    const caja = await boton.boundingBox();
    expect(caja, `no se encontró el control "${nombre}"`).not.toBeNull();
    expect(caja!.height, `"${nombre}" mide ${caja!.height}px de alto y el piso es 44`).toBeGreaterThanOrEqual(44);
    expect(caja!.width, `"${nombre}" mide ${caja!.width}px de ancho y el piso es 44`).toBeGreaterThanOrEqual(44);
  }
  // La papelera va al extremo opuesto del renglón (a la izquierda del nombre; −/+ a la derecha).
  const cajaMas = (await mas.boundingBox())!;
  const cajaQuitar = (await quitar.boundingBox())!;
  const hueco = Math.max(cajaQuitar.x - (cajaMas.x + cajaMas.width), cajaMas.x - (cajaQuitar.x + cajaQuitar.width));
  expect(hueco, 'la papelera quedó pegada al «+»').toBeGreaterThan(40);
  // Y el renglón mide un solo control de alto, no dos.
  const renglon = (await quitar.locator('..').boundingBox())!;
  expect(renglon.height, `el renglón nuevo mide ${renglon.height}px`).toBeLessThanOrEqual(64);
});

// T-cuenta · DESDE EL COBRO SE IMPRIME LA CUENTA DEL PEDIDO, Y LA HOJA SIGUE CABIENDO.
//
// Con una sola puerta, «Enviar y cobrar» manda a cocina antes de abrir la hoja: el papel ya es el
// de un pedido, con su número. El botón «Cuenta» mide 44 px.
test('T-cuenta · el papel de la cuenta sale desde el cobro y la hoja cabe en 600 px', async ({ page }) => {
  await entrar(page);
  await ponerUnProducto(page);
  await abrirTicket(page);
  await botonCobrar(page).click();
  await expect(page.getByRole('button', { name: 'Efectivo' })).toBeVisible({ timeout: 30_000 });

  const altoHoja = await altoDeLaHoja(page);
  expect(altoHoja, 'la hoja de cobro dejó de caber en la tableta').toBeLessThanOrEqual(600);
  const boton = page.getByRole('dialog').last().getByRole('button', { name: /^Cuenta$/ });
  const caja = await boton.boundingBox();
  expect(caja!.height, `el botón mide ${caja!.height}px y el piso es 44`).toBeGreaterThanOrEqual(44);
  await boton.click();
  const papel = page.frameLocator('iframe').first();
  await expect(papel.getByText(/#\d+/).first(), 'el papel de un pedido enviado lleva su número').toBeVisible({ timeout: 30_000 });
});

// E7 BIS · DIVIDIR POR PRODUCTOS CABE EN LA TABLETA (spec 027, D-13), ahora desde el ticket.
test('E7 bis · por productos, con efectivo y propina, Cobrar sigue a la vista', async ({ page }) => {
  await entrar(page);
  for (const p of ['Dedos de Queso Pza', 'Coca Cola 355ml', 'Chai Miel', 'Kit Kat']) {
    await ponerUnProducto(page, p);
  }
  await abrirTicket(page);
  await botonEnviar(page).click();
  // La cuenta enviada se queda abierta (caso 5): se cobra desde el mismo ticket.
  await expect(page.getByRole('heading', { name: /En cocina/ })).toBeVisible({ timeout: 20_000 });
  await botonCobrar(page).click();
  const hoja = page.getByRole('dialog').last();
  await hoja.getByRole('button', { name: /Dividir/ }).click();
  await hoja.getByRole('button', { name: 'Por productos' }).click();
  await hoja.getByRole('button', { name: 'Coca Cola 355ml' }).click();
  await hoja.getByRole('button', { name: 'Efectivo' }).click();
  await hoja.getByRole('button', { name: /^10%/ }).first().click().catch(() => {});

  const cobrar = hoja.getByRole('button', { name: /^Cobrar/ });
  const altoHoja = Math.round((await hoja.boundingBox())?.height ?? 0);
  expect(altoHoja, 'la hoja por productos no cabe en 600 px').toBeLessThanOrEqual(600);
  await expect(cobrar).toBeInViewport();
  await page.setViewportSize({ width: 1024, height: 330 });
  await expect(cobrar, 'con el teclado abierto, «Cobrar» salió de la pantalla').toBeInViewport();
  await page.setViewportSize({ width: 1024, height: 600 });
});

// ---------------------------------------------------------------------------------------------
// LA FILA DE CUENTAS, EL TICKET EN TRES SECCIONES Y LAS HOJAS NUEVAS (spec 030, T091; caso 29).
// ---------------------------------------------------------------------------------------------

test('F1 · la fila: fichas completas, «+N» cuenta las demás y nada se desborda con 10 cuentas', async ({ page }) => {
  const jwt = await tokenDeApi();
  const producto = await productoPorNombre(jwt, 'Dedos de Queso Pza');
  for (let i = 0; i < 10; i++) await crearCuentaPorApi(jwt, producto);
  await entrar(page);
  await expect(fichas(page).first()).toBeVisible({ timeout: 30_000 });

  const caja = (await fila(page).boundingBox())!;
  const vistas = await fichas(page).count();
  // Con el panel del ticket abierto la fila tiene ~612 px: al menos 2 fichas. Cerrado, al menos 4.
  const minimo = (await panelAbierto(page)) ? 2 : 4;
  expect(vistas, `solo se ven ${vistas} fichas`).toBeGreaterThanOrEqual(minimo);
  for (let i = 0; i < vistas; i++) {
    const f = (await fichas(page).nth(i).boundingBox())!;
    expect(f.x + f.width, 'una ficha se sale de la fila').toBeLessThanOrEqual(caja.x + caja.width + 1);
    expect(f.height, 'una ficha mide menos de 44 px').toBeGreaterThanOrEqual(44);
  }
  const mas = page.getByRole('button', { name: /^Ver todas las cuentas/ });
  await expect(mas).toBeInViewport();
  await expect(cuentaNueva(page)).toBeInViewport();
  expect(await alto(page, /^Ver todas las cuentas/)).toBeGreaterThanOrEqual(44);
  // Sin scroll horizontal escondido en la página.
  const desborda = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
  expect(desborda, 'la página tiene scroll horizontal').toBe(false);
  console.log(`[e2e] fila: ${vistas} fichas en ${Math.round(caja.width)}px (panel ${minimo === 2 ? 'abierto' : 'cerrado'})`);
});

test('F2 · la hoja «+N» cabe y sus renglones miden 56 px', async ({ page }) => {
  await entrar(page);
  await page.getByRole('button', { name: /^Ver todas las cuentas/ }).click();
  const hoja = page.getByRole('dialog').last();
  await expect(hoja.getByText('Cuentas', { exact: true })).toBeVisible({ timeout: 20_000 });
  expect(Math.round((await hoja.boundingBox())?.height ?? 0)).toBeLessThanOrEqual(600);
  const renglon = hoja.getByRole('button', { name: / · / }).first();
  if (await renglon.isVisible().catch(() => false)) {
    expect((await renglon.boundingBox())!.height).toBeGreaterThanOrEqual(56);
  }
});

test('F3 · el ticket con las tres secciones deja ver renglones y el pie', async ({ page }) => {
  await entrar(page);
  for (const p of ['Dedos de Queso Pza', 'Coca Cola 355ml', 'Chai Miel', 'Kit Kat']) {
    await ponerUnProducto(page, p);
  }
  await abrirTicket(page);
  await botonEnviar(page).click();
  await expect(page.getByRole('heading', { name: /En cocina/ })).toBeVisible({ timeout: 20_000 });
  // Cobrar uno por productos para que haya «Pagado».
  await botonCobrar(page).click();
  const hoja = page.getByRole('dialog').last();
  await hoja.getByRole('button', { name: /Dividir/ }).click();
  await hoja.getByRole('button', { name: 'Por productos' }).click();
  await hoja.getByRole('button', { name: 'Kit Kat' }).click();
  await hoja.getByRole('button', { name: 'Efectivo' }).click();
  await hoja.getByRole('button', { name: /^Cobrar \$/ }).click();
  await page.keyboard.press('Escape');
  // Y algo nuevo.
  await ponerUnProducto(page, 'Coca Cola 355ml');
  await abrirTicket(page);
  await expect(page.getByRole('heading', { name: /Nuevo · aún no va a cocina/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: /Pagado/ })).toBeVisible();

  // ≥ 4 renglones a la vista: lo nuevo con −/+ y lo enviado compacto.
  // Cada renglón es un hijo directo de su sección, después del encabezado.
  const renglones = page.getByRole('region').locator(':scope > :not(h3)');
  let visibles = 0;
  for (let i = 0; i < await renglones.count(); i++) {
    const c = await renglones.nth(i).boundingBox();
    if (c && c.y >= 0 && c.y + c.height <= 600) visibles++;
  }
  expect(visibles, `solo se ven ${visibles} renglones del ticket`).toBeGreaterThanOrEqual(4);
  await expect(botonCobrar(page)).toBeInViewport();
  await expect(botonEnviar(page)).toBeInViewport();
});

test('F4 · la hoja de descartar cabe con su botón a la vista', async ({ page }) => {
  await entrar(page);
  await ponerUnProducto(page);
  await abrirTicket(page);
  await page.getByRole('button', { name: 'Más opciones de la cuenta' }).click();
  await page.getByRole('menuitem', { name: /Descartar cuenta/ }).click();
  const hoja = page.getByRole('dialog').last();
  await expect(hoja.getByText(/¿Descartar la cuenta de/)).toBeVisible();
  for (const nombre of ['Seguir capturando', 'Descartar']) {
    const b = hoja.getByRole('button', { name: nombre, exact: true });
    await expect(b).toBeInViewport();
    expect((await b.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  }
  await hoja.getByRole('button', { name: 'Descartar', exact: true }).click();
});

test('F5 · el aviso sin conexión no mueve nada: va encima', async ({ page, context }) => {
  await entrar(page);
  const antes = (await cuentaNueva(page).boundingBox())!;
  await context.setOffline(true);
  await expect(page.getByRole('alert').filter({ hasText: 'Sin conexión' })).toBeVisible();
  const despues = (await cuentaNueva(page).boundingBox())!;
  expect(despues.y, 'el aviso empujó la fila').toBe(antes.y);
  await context.setOffline(false);
  await expect(page.getByRole('alert').filter({ hasText: 'Sin conexión' })).toHaveCount(0, { timeout: 20_000 });
});

test('F6 · el cierre de caja con cuentas vivas deja ver «Cerrar caja»', async ({ page }) => {
  const jwt = await tokenDeApi();
  const producto = await productoPorNombre(jwt, 'Dedos de Queso Pza');
  await crearCuentaPorApi(jwt, producto);
  await entrar(page);
  await page.goto('/caja');
  const plegada = page.getByRole('button', { name: /Cuentas pendientes \(\d+\)/ });
  await expect(plegada).toBeVisible({ timeout: 30_000 });
  expect((await plegada.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await plegada.click();
  const abrir = page.getByRole('button', { name: /^Abrir / }).first();
  const descartar = page.getByRole('button', { name: /^Descartar / }).first();
  expect((await abrir.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  expect((await descartar.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  const cerrar = page.getByRole('button', { name: 'Cerrar caja' });
  await cerrar.scrollIntoViewIfNeeded();
  await expect(cerrar).toBeInViewport();
});
