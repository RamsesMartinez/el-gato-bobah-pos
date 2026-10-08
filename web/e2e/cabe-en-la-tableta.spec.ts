import { test, expect, type Page } from '@playwright/test';
import { API, tokenDeApi } from './ambiente';

// LA HOJA DE COBRO TIENE QUE CABER EN LA TABLETA.
//
// El presupuesto real es 1024x600 y el alto es lo que escasea: cada control que se agrega a esta
// hoja se lo quita a lo que el operador vino a leer. Es un requisito funcional del producto y se
// olvida solo, así que se mide en vez de recordarse.
//
// Aquí se mide lo que NO se puede medir en vitest: el alto que ocupan de verdad los métodos de pago
// del negocio, que son tantos como ese negocio tenga configurados.

const USUARIO = process.env.E2E_USER ?? 'admin';
const EMPRESA = process.env.E2E_SLUG ?? 'gatobobah';
const PASSWORD = process.env.E2E_PASSWORD ?? 'Dev-ffb903b3dfb31073!';

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


// ponerUnProductoEnLaCuenta llega al producto por el BUSCADOR, no por el mosaico.
//
// Tocarlo directo funcionaba mientras el ambiente de pruebas tenía un catálogo sembrado y chico:
// el producto estaba a la vista al entrar. Con el catálogo real del negocio —cientos de productos
// repartidos en categorías— deja de estarlo, y siete casos se caían esperando 60 segundos a un
// texto que sí existe pero no está en pantalla. El buscador lo alcanza sin importar cuántos haya.
//
// Se busca SIEMPRE el mismo producto a propósito: varios de estos casos afirman importes, y tomar
// "el primero que aparezca" los volvería dependientes de qué catálogo tenga el ambiente.
async function ponerUnProductoEnLaCuenta(page: Page, nombre = 'Dedos de Queso Pza') {
  const buscador = page.getByPlaceholder('Buscar producto…');
  if (await buscador.isVisible().catch(() => false)) {
    await buscador.fill(nombre);
  }
  await page.getByText(nombre).first().click({ timeout: 30_000 });
}

test('E7 · la hoja de cobro cabe en 600 px, con y sin repartir', async ({ page }) => {
  await entrar(page);
  await ponerUnProductoEnLaCuenta(page);
  const confirmar = page.getByRole('button', { name: /^(Agregar|Confirmar)/ });
  if (await confirmar.isVisible().catch(() => false)) await confirmar.click();
  const pildora = page.getByRole('button', { name: /art ·/ });
  if (await pildora.isVisible().catch(() => false)) await pildora.click();
  await page.getByRole('button', { name: 'COBRAR' }).click();

  // Se espera a los métodos de pago: la hoja crece cuando llegan, y medir antes da un alto que el
  // operador nunca ve.
  await expect(page.getByRole('button', { name: 'Efectivo' })).toBeVisible({ timeout: 30_000 });
  const alto = async () => Math.round(
    (await page.locator('[role="dialog"]').first().boundingBox())?.height ?? 0);

  const sinRepartir = await alto();
  expect(sinRepartir, 'la hoja no cabe en la tableta sin repartir').toBeLessThanOrEqual(600);

  await page.getByRole('button', { name: /Dividir/ }).click();
  const repartiendo = await alto();
  expect(repartiendo, 'la hoja no cabe en la tableta al repartir').toBeLessThanOrEqual(600);

  // EL REPARTIDOR NO ES GRATIS, y por eso no vive abierto.
  //
  // Antes eran cuatro botones fijos —Todo, entre 2, 3 y 4— presentes en TODO cobro, y casi todos
  // los pedidos se le cobran a una sola persona. Esta diferencia es lo que la hoja dejó de pagar
  // en el caso común; si algún día vuelve a cero, es que el repartidor volvió a estar siempre.
  expect(repartiendo - sinRepartir,
    'el repartidor dejó de costar alto: ¿volvió a estar siempre abierto?').toBeGreaterThan(40);
  console.log(`[e2e] hoja de cobro: ${sinRepartir}px sin repartir, ${repartiendo}px repartiendo, de 600px`);
});

// X7 · LOS CONTROLES DEL RENGLÓN DEL TICKET, MEDIDOS EN PÍXELES REALES.
//
// Medían ~24 px, por debajo del piso de 44 que fija la constitución. Vitest no puede atrapar esto:
// las medidas de Chakra son clases CSS y jsdom no las resuelve, así que un assert de píxeles allá
// pasaría verde con los botones chicos. Aquí hay un navegador de verdad.
//
// Y se mide también la SEPARACIÓN con la papelera: estaba pegada al menos, así que quitar una
// unidad y borrar el renglón entero se distinguían por unos pocos píxeles.
test('X7 · los controles del renglón del ticket miden 44 px y la papelera va aparte', async ({ page }) => {
  await entrar(page);
  await ponerUnProductoEnLaCuenta(page);

  // El mismo camino que E7, y por las mismas razones: el producto puede abrir la hoja de
  // modificadores, y a 1024x600 el POS está en modo ANGOSTO — el ticket no es un panel lateral sino
  // una hoja inferior que se abre desde la barra. Darlo por hecho es lo que ya tumbó dos pruebas de
  // esta suite.
  const confirmar = page.getByRole('button', { name: /^(Agregar|Confirmar)/ });
  if (await confirmar.isVisible().catch(() => false)) await confirmar.click();
  const barra = page.getByRole('button', { name: /art ·/ });
  if (await barra.isVisible().catch(() => false)) await barra.click();
  await expect(page.getByRole('button', { name: 'Quitar' }).first()).toBeVisible({ timeout: 30_000 });

  const menos = page.getByRole('button', { name: '−' }).first();
  const mas = page.getByRole('button', { name: '+' }).first();
  const quitar = page.getByRole('button', { name: 'Quitar' }).first();

  for (const [nombre, boton] of [['−', menos], ['+', mas], ['Quitar', quitar]] as const) {
    const caja = await boton.boundingBox();
    expect(caja, `no se encontró el control "${nombre}"`).not.toBeNull();
    expect(caja!.height, `"${nombre}" mide ${caja!.height}px de alto y el piso es 44`).toBeGreaterThanOrEqual(44);
    expect(caja!.width, `"${nombre}" mide ${caja!.width}px de ancho y el piso es 44`).toBeGreaterThanOrEqual(44);
  }

  // La papelera al otro extremo: entre ella y el "+" tiene que haber más que el hueco de un gap.
  const cajaMas = (await mas.boundingBox())!;
  const cajaQuitar = (await quitar.boundingBox())!;
  const hueco = cajaQuitar.x - (cajaMas.x + cajaMas.width);
  expect(hueco, `la papelera está a ${Math.round(hueco)}px del "+": un toque impreciso borra el renglón`)
    .toBeGreaterThan(40);
});

// LA CUENTA SE IMPRIME DESDE EL COBRO, Y LA HOJA SIGUE CABIENDO.
//
// El papel de una cuenta sin confirmar lleva ** PRE-CUENTA ** y NO lleva número de pedido: no
// existe todavía. Es lo que impide que pase por un comprobante de venta, y solo se puede comprobar
// en un navegador de verdad porque el papel se pinta dentro de un iframe.
test('T-cuenta · el papel de la cuenta sale marcado y la hoja cabe en 600 px', async ({ page }) => {
  await entrar(page);
  await ponerUnProductoEnLaCuenta(page);
  const confirmar = page.getByRole('button', { name: /^(Agregar|Confirmar)/ });
  if (await confirmar.isVisible().catch(() => false)) await confirmar.click();
  const barra = page.getByRole('button', { name: /art ·/ });
  if (await barra.isVisible().catch(() => false)) await barra.click();
  await page.getByRole('button', { name: 'COBRAR' }).click();
  await expect(page.getByRole('button', { name: 'Efectivo' })).toBeVisible({ timeout: 30_000 });

  const alto = Math.round((await page.locator('[role="dialog"]').first().boundingBox())?.height ?? 0);
  expect(alto, 'la hoja de cobro dejó de caber en la tableta').toBeLessThanOrEqual(600);

  const boton = page.getByRole('button', { name: /Cuenta/ });
  const caja = await boton.boundingBox();
  expect(caja!.height, `el botón mide ${caja!.height}px y el piso es 44`).toBeGreaterThanOrEqual(44);
  await boton.click();

  const papel = page.frameLocator('iframe').first();
  await expect(papel.getByText('PRE-CUENTA'),
    'el papel de una cuenta sin confirmar tiene que decir que lo es').toBeVisible({ timeout: 30_000 });
  await expect(papel.getByText(/Pedido #/),
    'el papel trae un número de pedido que todavía no existe').toHaveCount(0);
  await expect(papel.getByText('POR COBRAR'),
    'el estado del cobro lo confunde con el ticket de un pedido real').toHaveCount(0);
  console.log(`[e2e] hoja de cobro con boton de cuenta: ${alto}px de 600px`);
});

// pedidosAbiertos lee del servidor los pedidos por cobrar, con el token de la sesión del navegador.
async function pedidosAbiertos(page: Page): Promise<Array<{ id: number; number: number }>> {
  const jwt = await tokenDeApi();
  const r = await page.request.get(`${API}/orders/open`, { headers: { Authorization: `Bearer ${jwt}` } });
  return ((await r.json()).items ?? []) as Array<{ id: number; number: number }>;
}

// E7 BIS · DIVIDIR POR PRODUCTOS CABE EN LA TABLETA (spec 027, D-13).
//
// La hoja crece por dentro: la lista, la propina y el efectivo van en la zona con scroll, y el pie
// con «Cobrar» se queda a la vista. Se mide a 600 px y con el teclado del sistema abierto, que en la
// tableta deja ~330 px: si el botón sale de la pantalla, el operador tiene que cerrar el teclado para
// cobrar, con el cliente enfrente. La vista de «Pasar a otro pedido» también deja su botón visible.
test('E7 bis · por productos, con efectivo y propina, Cobrar sigue a la vista', async ({ page }) => {
  await entrar(page);
  for (const p of ['Dedos de Queso Pza', 'Coca Cola 355ml', 'Chai Miel', 'Kit Kat']) {
    await ponerUnProductoEnLaCuenta(page, p);
    const confirmar = page.getByRole('button', { name: /^(Agregar|Confirmar)/ });
    if (await confirmar.isVisible().catch(() => false)) await confirmar.click();
    const buscador = page.getByPlaceholder('Buscar producto…');
    if (await buscador.isVisible().catch(() => false)) await buscador.fill('');
  }
  const antes = new Set((await pedidosAbiertos(page)).map((o) => o.id));
  const pildora = page.getByRole('button', { name: /art ·/ });
  if (await pildora.isVisible().catch(() => false)) await pildora.click();
  await page.getByRole('button', { name: 'Enviar a cocina' }).click();
  let numero = 0;
  await expect.poll(async () => {
    numero = (await pedidosAbiertos(page)).find((o) => !antes.has(o.id))?.number ?? 0;
    return numero;
  }, { timeout: 20_000 }).toBeGreaterThan(0);
  const nuevo = page.getByRole('button', { name: 'Nuevo pedido' });
  if (await nuevo.isVisible().catch(() => false)) await nuevo.click();

  // Desde «Pedidos por cobrar»: un pedido con algo por entregar se cobra desde ahí.
  await page.getByRole('button', { name: /^\$[\d,.]+ \(\d+\)$/ }).click();
  const lista = page.getByRole('dialog').last();
  await lista.locator('div').filter({ has: page.getByText(new RegExp(`^#${numero} · `)) })
    .filter({ has: page.getByRole('button', { name: /^Cobrar/ }) }).last()
    .getByRole('button', { name: /^Cobrar/ }).click();
  const hoja = page.getByRole('dialog').last();
  await hoja.getByRole('button', { name: /Dividir/ }).click();
  await hoja.getByRole('button', { name: 'Coca Cola 355ml' }).click();
  await hoja.getByRole('button', { name: 'Efectivo' }).click();
  await hoja.getByRole('button', { name: /^10%/ }).first().click().catch(() => {});

  const cobrar = hoja.getByRole('button', { name: /^Cobrar/ });
  const alto = Math.round((await hoja.boundingBox())?.height ?? 0);
  expect(alto, 'la hoja por productos no cabe en 600 px').toBeLessThanOrEqual(600);
  await expect(cobrar).toBeInViewport();

  // El teclado del sistema abierto: el viewport baja a ~330 px.
  await page.setViewportSize({ width: 1024, height: 330 });
  await expect(cobrar, 'con el teclado abierto, «Cobrar» salió de la pantalla').toBeInViewport();
  await page.setViewportSize({ width: 1024, height: 600 });

  await hoja.getByRole('button', { name: /Pasar a otro pedido/ }).click();
  const pasar = hoja.getByRole('button', { name: /^(Pasar a|Elige a qué pedido)/ });
  await expect(pasar, 'la vista de pasar no deja ver su botón').toBeInViewport();
  console.log(`[e2e] hoja por productos: ${alto}px de 600px`);
});
