import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
import { API } from './ambiente';

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
async function token(request: APIRequestContext): Promise<string> {
  const r = await request.post(`${API}/auth/login`, {
    data: { username: USUARIO, slug: EMPRESA, password: PASSWORD },
  });
  expect(r.ok(), 'el login del ambiente de pruebas falló').toBeTruthy();
  return (await r.json()).accessToken;
}

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
  // El catálogo es lo que confirma que el POS cargó. El botón de cobrar NO sirve de señal: en
  // 1024x600 el panel del pedido arranca colapsado para dejarle el ancho al catálogo, así que
  // COBRAR ni siquiera está en el árbol hasta que se abre.
  await expect(page.getByRole('button', { name: 'Cuenta 1' })).toBeVisible({ timeout: 30_000 });
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
  await expect(page.getByRole('button', { name: 'COBRAR' })).toBeVisible({ timeout: 15_000 });
}

// El primer producto con precio del catálogo. Se toma de la pantalla y no de una lista fija: el
// menú del ambiente de pruebas cambia, y un test atado a "Alitas" falla el día que alguien la
// renombra, por un motivo que no tiene nada que ver con el dinero.
async function agregarUnProducto(page: Page): Promise<void> {
  // Uno SIN modificadores: los que los tienen abren otra hoja y lo que esta suite mide es el cobro,
  // no el armado del pedido.
  await page.getByText('Dedos de Queso Pza').first().click();
  const confirmar = page.getByRole('button', { name: /^(Agregar|Confirmar)/ });
  if (await confirmar.isVisible().catch(() => false)) await confirmar.click();
  await verElPedido(page);
}

test.describe('E — el cobro, en la pantalla', () => {
  // TOCAR COBRAR NO MANDA NADA A COCINA.
  //
  // Antes creaba el pedido aquí mismo. El botón vive junto al total, en la barra que se toca todo el
  // día, así que un toque por equivocación dejaba comida preparándose y una cuenta que alguien tenía
  // que ir a cancelar. Ahora el pedido nace al tocar el botón final, el que dice cuánto se cobra.
  //
  // Se mide contra el SERVIDOR —cuántos pedidos en curso hay antes y después— porque es lo único
  // que distingue "no se creó" de "se creó y la pantalla no lo pintó".
  test('E1 · COBRAR abre la hoja y NO manda el pedido a cocina', async ({ page, request }) => {
    const jwt = await token(request);
    const cuantos = async () => {
      const r = await request.get(`${API}/orders/open`, { headers: { Authorization: `Bearer ${jwt}` } });
      return ((await r.json()).items ?? []).length as number;
    };
    const antes = await cuantos();

    await entrar(page);
    await agregarUnProducto(page);
    await page.getByRole('button', { name: 'COBRAR' }).click();

    // La hoja de cobro es la misma del botón naranja: se reconoce por el encabezado que dice las dos
    // cifras. La pantalla vieja del carrito decía "Cobrar · $X" y ya no existe.
    await expect(page.getByText(/Falta \$/)).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText(/Total \$/)).toBeVisible();

    // NINGÚN método viene preseleccionado, a propósito: un dedo que va directo a Cobrar registraría
    // con tarjeta dinero que entró en efectivo.
    await expect(page.getByText('Falta con qué paga.')).toBeVisible();
    await expect(page.getByRole('button', { name: /^Cobrar \$/ })).toBeDisabled();

    // Y lo que importa: cocina no se enteró de nada.
    expect(await cuantos(), 'tocar COBRAR mandó el pedido a cocina').toBe(antes);
  });

  test('E1b · cobrando en efectivo, el pedido queda saldado y sale de la barra', async ({ page }) => {
    await entrar(page);
    await agregarUnProducto(page);
    await page.getByRole('button', { name: 'COBRAR' }).click();
    await expect(page.getByText(/Falta \$/)).toBeVisible({ timeout: 30_000 });

    // El folio con el que se canta el pedido: es lo que después se busca en la barra.
    const cobrar = page.getByRole('button', { name: /^Cobrar \$/ });
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
      await page.getByRole('button', { name: 'Domicilio' }).click();
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

    await page.getByRole('button', { name: 'Domicilio' }).click();

    // La coma de millar que el operador teclea por costumbre. `parseFloat` la leía como 1 y el
    // resto la volvía cero: envío gratis que nadie decidió.
    await page.getByLabel('Costo de envío').fill('1,000');
    await expect(page.getByText('Solo números')).toBeVisible();
    await expect(page.getByRole('button', { name: 'COBRAR' })).toBeDisabled();
  });
});
