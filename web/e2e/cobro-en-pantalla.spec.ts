import { test, expect, type Page } from '@playwright/test';
import { pedidosEnCurso, tokenDeRequest } from './ambiente';
import { botonCobrar, buscar, cerrarBusqueda } from './pos';

// LA MATRIZ DE DINERO, PASANDO POR LA PANTALLA. Ver docs/matriz-de-cobro.md, sección E.
//
// Aquí no se prueba el contrato —eso está en dinero.spec.ts— sino el flujo que el operador recorre
// con el cliente enfrente. Es el único lugar donde se ve que el pedido sale de la barra, que el
// botón se apaga cuando debe, y que la pantalla y el servidor dicen la misma cifra.

const USUARIO = process.env.E2E_USER ?? 'admin';
const EMPRESA = process.env.E2E_SLUG ?? 'gatobobah';
const PASSWORD = process.env.E2E_PASSWORD ?? 'Dev-ffb903b3dfb31073!';

// El token para preguntarle al SERVIDOR qué pasó. La pantalla puede no pintar un pedido que sí se
// creó, y esa diferencia es justo la que hay que medir.

async function entrar(page: Page) {
  await page.goto('/');
  // Se espera a que la app hidrate ANTES de teclear: sin esto, el formulario se re-renderiza al
  // resolverse el intento de sesión y se lleva lo escrito.
  await page.waitForLoadState('networkidle');
  const usuario = page.getByPlaceholder('usuario@empresa');
  if (await usuario.isVisible().catch(() => false)) {
    await usuario.fill(`${USUARIO}@${EMPRESA}`);
    await page.getByPlaceholder('Contraseña').fill(PASSWORD);
    await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  }
  // El «+» de la fila de cuentas es lo que confirma que el POS cargó. El botón de cobrar NO sirve de
  // señal: en 1024x600 el panel del pedido arranca colapsado y Cobrar no está hasta que se abre.
  await expect(page.getByRole('button', { name: 'Cuenta nueva', exact: true })).toBeVisible({ timeout: 30_000 });
}

// Abre el panel del pedido.
//
// En 1024x600 arranca colapsado para dejarle el ancho al catálogo: con la cuenta vacía queda un
// botón "Ver pedido", y con productos capturados una píldora flotante que dice cuántos artículos
// lleva. Los dos caminos abren el mismo panel.
async function verElPedido(page: Page) {
  const pildora = page.getByRole('button', { name: /art ·/ });
  if (await pildora.isVisible().catch(() => false)) await pildora.click();
  else {
    const abrir = page.getByRole('button', { name: /Ver pedido/ });
    if (await abrir.isVisible().catch(() => false)) await abrir.click();
  }
  await expect(botonCobrar(page)).toBeVisible({ timeout: 15_000 });
}

// El primer producto con precio del catálogo. Se toma de la pantalla y no de una lista fija: el
// menú del ambiente de pruebas cambia, y un test atado a "Alitas" falla el día que alguien la
// renombra, por un motivo que no tiene nada que ver con el dinero.
async function agregarUnProducto(page: Page): Promise<void> {
  // Uno SIN modificadores: los que los tienen abren otra hoja y lo que esta suite mide es el cobro,
  // no el armado del pedido.
  await buscar(page, 'Dedos de Queso Pza');
  await page.getByText('Dedos de Queso Pza').first().click();
  const confirmar = page.getByRole('button', { name: /^(Agregar|Confirmar)/ });
  if (await confirmar.isVisible().catch(() => false)) await confirmar.click();
  await expect(page.getByText('Guardando…')).toHaveCount(0, { timeout: 15_000 });
  await cerrarBusqueda(page);
  await verElPedido(page);
}

// aDomicilio: el tipo de servicio es UN botón en la cabecera del ticket que dice el tipo ACTUAL
// («Mostrador») y lo alterna. Hay otro «Mostrador» —el selector de plataforma, arriba del menú—, y el
// del ticket es el último en la página.
async function aDomicilio(page: Page) {
  await page.getByRole('button', { name: 'Mostrador', exact: true }).last().click();
  await expect(page.getByRole('button', { name: 'Domicilio', exact: true })).toBeVisible();
}

test.describe('E — el cobro, en la pantalla', () => {
  // «ENVIAR Y COBRAR» LO DICE, MANDA A COCINA PRIMERO, Y CERRAR LA HOJA NO CANCELA NADA (US4, caso 11).
  //
  // Con una sola puerta, cobrar una cuenta que tiene algo sin enviar lo manda a cocina antes de
  // abrir la hoja: «Por productos» necesita los renglones del pedido (research R-5). El botón lo
  // dice, y quien cierra la hoja sin cobrar encuentra la cuenta en la fila, en cocina, no perdida.
  //
  // Se mide contra el SERVIDOR: es lo único que distingue «se mandó» de «la pantalla lo pintó».
  test('E1 · «Enviar y cobrar» manda a cocina antes de abrir la hoja, y cerrarla no cancela', async ({ page, request }) => {
    const jwt = await tokenDeRequest(request);
    const antes = new Set((await pedidosEnCurso(jwt)).map((o) => o.id));

    await entrar(page);
    await agregarUnProducto(page);
    const boton = botonCobrar(page);
    await expect(boton).toHaveText(/^Enviar y cobrar/);
    await boton.click();

    await expect(page.getByText(/Falta \$/)).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText(/Total \$/)).toBeVisible();
    // NINGÚN método viene preseleccionado: un dedo que va directo a Cobrar registraría con tarjeta
    // dinero que entró en efectivo.
    await expect(page.getByText('Falta con qué paga.')).toBeVisible();
    await expect(page.getByRole('dialog').last().getByRole('button', { name: /^Cobrar \$/ })).toBeDisabled();

    const nuevos = (await pedidosEnCurso(jwt)).filter((o) => !antes.has(o.id));
    expect(nuevos, '«Enviar y cobrar» no mandó la cuenta a cocina').toHaveLength(1);

    await page.keyboard.press('Escape');
    // La cuenta sigue en la fila, ya en cocina, y en el servidor nada se canceló.
    await expect(page.getByRole('button', { name: new RegExp(`^${nuevos[0].folioName} · En cocina`) })).toBeVisible({ timeout: 20_000 });
    expect((await pedidosEnCurso(jwt)).some((o) => o.id === nuevos[0].id), 'cerrar la hoja canceló el pedido').toBe(true);
  });

  test('E1b · cobrando en efectivo, el pedido queda saldado y sale de la barra', async ({ page }) => {
    await entrar(page);
    await agregarUnProducto(page);
    await botonCobrar(page).click();
    await expect(page.getByText(/Falta \$/)).toBeVisible({ timeout: 30_000 });

    const cobrar = page.getByRole('dialog').last().getByRole('button', { name: /^Cobrar \$/ });
    await page.getByRole('button', { name: 'Efectivo', exact: true }).click();
    await expect(cobrar).toBeEnabled();
    await cobrar.click();

    // La confirmación de la venta. Con el pedido saldado dice Cobrado, no "falta".
    await expect(page.getByText(/^Cobrado ·/)).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText(/Falta cobrar/)).toHaveCount(0);
  });

  test('E5b · con plataforma la pantalla no ofrece cobrar un envío que el servidor no cobra',
    async ({ page }) => {
      await entrar(page);
      await agregarUnProducto(page);

      // Se marca domicilio PRIMERO y la plataforma después: es la secuencia que dejaba la cuenta en
      // domicilio con plataforma, sumando $20 que el servidor fuerza a 0. El panel esconde los
      // botones de tipo en cuanto hay plataforma, así que ya no se puede corregir a mano.
      // Se afirma que el campo APARECE antes de asignar la plataforma. Sin esta comprobación el
      // test pasaría por vacío el día que el campo deje de existir por otra razón.
      await aDomicilio(page);
      await expect(page.getByLabel('Costo de envío')).toBeVisible();

      // El panel se cierra para llegar al selector de plataforma, que vive en la barra de arriba.
      await page.getByLabel('Ocultar pedido').click();
      await page.getByRole('button', { name: 'Uber Eats' }).click();
      await verElPedido(page);

      // Con plataforma, el campo de envío desaparece: el reparto lo cobra ella, y ofrecerlo era
      // cobrar $20 que el servidor fuerza a 0.
      await expect(page.getByLabel('Costo de envío')).toHaveCount(0);
    });

  test('E6 · un envío mal escrito no se convierte en envío gratis', async ({ page }) => {
    await entrar(page);
    await agregarUnProducto(page);

    await aDomicilio(page);

    // La coma de millar que el operador teclea por costumbre. `parseFloat` la leía como 1 y el
    // resto la volvía cero: envío gratis que nadie decidió.
    await page.getByLabel('Costo de envío').fill('1,000');
    await expect(page.getByText('Solo números')).toBeVisible();
    await expect(botonCobrar(page)).toBeDisabled();
  });
});
