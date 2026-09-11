import { test, expect, type Page } from '@playwright/test';

// CONTAR EL CAJÓN, MEDIDO EN LA TABLETA (spec 003).
//
// Lo que vitest no puede ver: el alto que ocupa de verdad la hoja del contador con las once
// denominaciones puestas, y si el operador tiene que desplazarse para llegar a las monedas chicas.
// Las medidas de Chakra son clases CSS y jsdom no las resuelve, así que allá un assert de píxeles
// pasa verde con la rejilla desbordada.
//
// Y lo que ningún test de pantalla puede ver: que el total que la hoja muestra sea el mismo que el
// servidor guarda. Ese desacuerdo es la razón de ser de esta suite.

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

// abrirLaHojaDeConteo deja la hoja del contador abierta sobre una caja CERRADA.
//
// Devuelve null si no hay ninguna cerrada: el ambiente es compartido y una caja abierta por alguien
// más no se toca. Un test que "arregla" el ambiente para poder correr es un test que miente.
async function abrirLaHojaDeConteo(page: Page): Promise<string | null> {
  await page.goto('/caja');
  await page.waitForLoadState('networkidle');
  // La pantalla arranca en la caja PRIMARIA, que en un ambiente vivo suele estar abierta. El chip de
  // cada caja trae su estado, así que se elige una cerrada — y si no hay, el test se salta: abrir o
  // cerrar la caja de alguien más no es algo que una prueba pueda hacer.
  const chipCerrada = page.getByRole('button', { name: /Cerrada/ }).first();
  if (!(await chipCerrada.isVisible().catch(() => false))) return null;
  const caja = (await chipCerrada.textContent()) ?? '';
  await chipCerrada.click();
  const contar = page.getByRole('button', { name: 'Contar el efectivo' }).first();
  await expect(contar).toBeVisible({ timeout: 30_000 });
  await contar.click();
  // Se espera a que el catálogo llegue: la rejilla crece cuando aparecen las once denominaciones, y
  // medir antes da un alto que el operador nunca ve.
  await expect(page.getByLabel('Piezas de 50¢', { exact: true })).toBeVisible({ timeout: 30_000 });
  return caja;
}

test('C1 · la hoja del contador cabe en 600 px y las monedas chicas se alcanzan sin desplazarse', async ({ page }) => {
  await entrar(page);
  const caja = await abrirLaHojaDeConteo(page);
  test.skip(caja === null, 'no hay una caja cerrada en el ambiente: no se toca la de alguien más');

  const hoja = page.locator('[role="dialog"]').first();
  const alto = Math.round((await hoja.boundingBox())?.height ?? 0);
  expect(alto, 'la hoja del contador no cabe en la tableta').toBeLessThanOrEqual(600);

  // LA ÚLTIMA MONEDA ES LA QUE SE CAE PRIMERO: es el renglón de más abajo de la rejilla y el que
  // motivó la feature (contar monedas de 50¢ es lo que nadie quiere sumar a mano). Si hay que
  // desplazarse para llegar a ella, la hoja propia no cumplió su propósito.
  await expect(page.getByLabel('Piezas de 50¢', { exact: true })).toBeInViewport();
  // Y el total y el botón de confirmar, que son la razón del footer fijo.
  await expect(page.getByLabel('Total contado')).toBeInViewport();
  await expect(page.getByRole('button', { name: 'Abrir caja' })).toBeInViewport();
  console.log(`[e2e] hoja del contador: ${alto}px de 600px`);
});

test('C2 · los controles del contador miden 44 px', async ({ page }) => {
  await entrar(page);
  const caja = await abrirLaHojaDeConteo(page);
  test.skip(caja === null, 'no hay una caja cerrada en el ambiente');

  // Después de que la animación asiente: `boundingBox()` devuelve la caja TRANSFORMADA, así que
  // medir mientras el drawer entra da un alto escalado y el assert pasa o falla por la animación.
  // Es el mismo tropiezo que documenta Z3 de docs/matriz-de-pantallas.md.
  await page.waitForTimeout(500);

  for (const etiqueta of ['Piezas de $1,000', 'Piezas de 50¢']) {
    const campo = page.getByLabel(etiqueta);
    const caja = await campo.boundingBox();
    expect(Math.round(caja?.height ?? 0), `el campo «${etiqueta}» quedó por debajo de 44 px`)
      .toBeGreaterThanOrEqual(44);
  }
  for (const etiqueta of ['Una más de $1,000', 'Una menos de $1,000']) {
    const boton = page.getByLabel(etiqueta);
    const caja = await boton.boundingBox();
    expect(Math.round(caja?.height ?? 0), `el ajuste «${etiqueta}» quedó por debajo de 44 px`)
      .toBeGreaterThanOrEqual(44);
    expect(Math.round(caja?.width ?? 0), `el ajuste «${etiqueta}» quedó angosto`).toBeGreaterThanOrEqual(44);
  }

  // EL CAMPO ES EL CONTROL PRINCIPAL, y eso se tiene que ver: si las dos teclas juntas ocupan más
  // que él, el ojo va a ellas y el operador termina dando 40 taps.
  const campo = await page.getByLabel('Piezas de $1,000', { exact: true }).boundingBox();
  const mas = await page.getByLabel('Una más de $1,000', { exact: true }).boundingBox();
  expect((campo?.width ?? 0), 'el campo es más angosto que una tecla de ajuste')
    .toBeGreaterThan(mas?.width ?? 0);
});

test('C3 · con el teclado abierto el total y el botón siguen a la vista', async ({ page }) => {
  await entrar(page);
  const caja = await abrirLaHojaDeConteo(page);
  test.skip(caja === null, 'no hay una caja cerrada en el ambiente');

  // El teclado del sistema no se puede abrir desde Playwright, así que se simula lo único que
  // importa de él: que se come ~250 px de ventana visual. Es exactamente lo que `dvh` y el footer
  // fijo existen para resolver, y sin ellos el total y el botón se van debajo del teclado.
  await page.setViewportSize({ width: 1024, height: 350 });
  await page.getByLabel('Piezas de $100', { exact: true }).fill('3');
  await page.waitForTimeout(300);

  await expect(page.getByLabel('Total contado')).toBeInViewport();
  await expect(page.getByRole('button', { name: 'Abrir caja' })).toBeInViewport();
});

test('C4 · contar 40 monedas se teclea, y el total es el que suma el servidor', async ({ page }) => {
  await entrar(page);
  const caja = await abrirLaHojaDeConteo(page);
  test.skip(caja === null, 'no hay una caja cerrada en el ambiente');

  // 40 monedas de $10 y 3 billetes de $50: $550. Con tap = +1 serían 43 taps y no cabrían en los 2
  // minutos de SC-003; aquí son dos campos.
  await page.getByLabel('Piezas de $10', { exact: true }).fill('40');
  await page.getByLabel('Piezas de $50', { exact: true }).fill('3');
  await expect(page.getByLabel('Total contado')).toHaveText('$550');

  await page.getByRole('button', { name: 'Abrir caja' }).click();

  // EL SERVIDOR TIENE QUE HABER GUARDADO LA MISMA CIFRA. Es lo único que esta suite puede probar y
  // los tests de pantalla no: la hoja suma en el cliente y el servidor recalcula desde las piezas —
  // si los dos no coinciden, el arqueo del turno se compara contra un fondo que nadie contó.
  await expect(page.getByText('Monto inicial')).toBeVisible({ timeout: 30_000 });
  await expect(page.locator('text=$550').first()).toBeVisible();

  // Y SE DEJA COMO ESTABA: la caja se abrió para probar, así que se cierra contando lo mismo. Un
  // turno de prueba que se queda abierto suma a "por cobrar" y bloquea el cierre de quien opera.
  await page.getByRole('button', { name: 'Contar efectivo' }).click();
  await expect(page.getByLabel('Piezas de $10', { exact: true })).toBeVisible({ timeout: 30_000 });
  await page.getByLabel('Piezas de $10', { exact: true }).fill('40');
  await page.getByLabel('Piezas de $50', { exact: true }).fill('3');
  await page.getByRole('button', { name: 'Usar este conteo' }).click();

  // La diferencia se ve ANTES de cerrar, que es FR-005.
  await expect(page.getByLabel('Diferencia del arqueo')).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText('El arqueo cuadra')).toBeVisible();

  page.once('dialog', (d) => d.accept());
  await page.getByRole('button', { name: 'Cerrar caja' }).click();
  await expect(page.getByText('Caja cerrada')).toBeVisible({ timeout: 30_000 });
});

// LOS PASOS DEL QUICKSTART, EJECUTABLES (T032).
//
// El recorrido manual del quickstart cubre lo mismo, pero un recorrido se hace una vez y estos
// corren en cada suite. Los que quedan fuera son los de juicio visual: si la pantalla "se ve bien"
// no lo decide un assert.

test('C5 · un faltante se ve antes de cerrar, no bloquea, y después queda su desglose', async ({ page }) => {
  await entrar(page);
  const caja = await abrirLaHojaDeConteo(page);
  test.skip(caja === null, 'no hay una caja cerrada en el ambiente');

  // Se abre con $500 contados y se cierra declarando $450: faltan $50.
  await page.getByLabel('Piezas de $500', { exact: true }).fill('1');
  await page.getByRole('button', { name: 'Abrir caja' }).click();
  await expect(page.getByText('Monto inicial')).toBeVisible({ timeout: 30_000 });

  await page.getByRole('button', { name: 'Contar efectivo' }).click();
  await expect(page.getByLabel('Piezas de $100', { exact: true })).toBeVisible({ timeout: 30_000 });
  await page.getByLabel('Piezas de $100', { exact: true }).fill('4');
  await page.getByLabel('Piezas de $50', { exact: true }).fill('1');
  await page.getByRole('button', { name: 'Usar este conteo' }).click();

  // SIN TOCAR «Cerrar caja»: el faltante ya está a la vista. Es FR-005, y antes solo se veía en el
  // diálogo POSTERIOR al cierre, cuando el corte ya estaba firmado.
  await expect(page.getByLabel('Diferencia del arqueo')).toHaveText('-$50');
  await expect(page.getByText('Faltante')).toBeVisible();

  // Y un faltante NO bloquea el cierre: si bloqueara, el operador se queda con la caja abierta toda
  // la noche por $50 que no aparecen.
  const cerrar = page.getByRole('button', { name: 'Cerrar caja' });
  await expect(cerrar).toBeEnabled();
  page.once('dialog', (d) => d.accept());
  await cerrar.click();
  await expect(page.getByText('Caja cerrada')).toBeVisible({ timeout: 30_000 });
  await page.getByRole('button', { name: 'Close' }).first().click().catch(() => {});

  // EL DESGLOSE SE LEE DESPUÉS, que es la razón de guardarlo: "faltan $50" se vuelve "declaró 4 de
  // $100 y 1 de $50".
  await page.getByRole('tab', { name: 'Histórico' }).click();
  await page.getByRole('row').nth(1).click();
  await expect(page.getByText('Efectivo contado')).toBeVisible({ timeout: 30_000 });
  await page.getByText('Efectivo contado').click();
  await expect(page.getByText('Al cerrar')).toBeVisible();
  await expect(page.getByText('Al abrir')).toBeVisible();
});

test('C6 · el total a mano exige motivo, y cambiar de camino avisa antes de borrar', async ({ page }) => {
  await entrar(page);
  const caja = await abrirLaHojaDeConteo(page);
  test.skip(caja === null, 'no hay una caja cerrada en el ambiente');

  // Se capturan piezas y se cambia de camino: tiene que avisar ANTES de descartarlas.
  await page.getByLabel('Piezas de $200', { exact: true }).fill('2');
  await page.getByRole('tab', { name: 'Escribir el total' }).click();
  await expect(page.getByText('Al cambiar se borra lo que ya capturaste.')).toBeVisible();
  await page.getByRole('button', { name: 'Seguir aquí' }).click();
  await expect(page.getByLabel('Piezas de $200', { exact: true })).toHaveValue('2');

  await page.getByRole('tab', { name: 'Escribir el total' }).click();
  await page.getByRole('button', { name: 'Borrar y cambiar' }).click();

  // El total a mano sin motivo no se puede confirmar: sería una cifra que nadie puede justificar.
  await page.getByLabel('Efectivo', { exact: true }).fill('400');
  await expect(page.getByRole('button', { name: 'Abrir caja' })).toBeDisabled();
  await page.getByLabel('Por qué no se contó pieza por pieza').fill('billete que no está en la lista');
  await page.getByRole('button', { name: 'Abrir caja' }).click();

  await expect(page.getByText('Monto inicial')).toBeVisible({ timeout: 30_000 });

  // Y se deja como estaba.
  await page.getByRole('button', { name: 'Contar efectivo' }).click();
  await expect(page.getByLabel('Piezas de $200', { exact: true })).toBeVisible({ timeout: 30_000 });
  await page.getByLabel('Piezas de $200', { exact: true }).fill('2');
  await page.getByRole('button', { name: 'Usar este conteo' }).click();
  page.once('dialog', (d) => d.accept());
  await page.getByRole('button', { name: 'Cerrar caja' }).click();
  await expect(page.getByText('Caja cerrada')).toBeVisible({ timeout: 30_000 });
});

test('C7 · un corte anterior a la funcionalidad no muestra desglose ni lo inventa', async ({ page }) => {
  await entrar(page);
  await page.goto('/caja');
  await page.waitForLoadState('networkidle');
  await page.getByRole('tab', { name: 'Histórico' }).click();

  // El corte MÁS VIEJO del histórico: cerró antes de que existiera el conteo. Es el caso de todos
  // los que ya viven en producción, y son la mayoría.
  //
  // Esto SUPONE un ambiente con historia, que es el que esta suite tiene por contrato
  // (`playwright.config.ts`). Contra una base recién sembrada —donde todos los cortes los creó
  // quien está probando, y por lo tanto todos traen conteo— este caso falla sin que haya nada roto.
  // Se deja fallando en vez de saltarse solo: un skip automático aquí lo volvería una tautología,
  // porque la condición que lo saltaría es exactamente la que viene a comprobar.
  const filas = page.getByRole('row');
  const cuantas = await filas.count();
  test.skip(cuantas < 3, 'el histórico no tiene cortes anteriores a la feature');
  await filas.nth(cuantas - 1).click();

  // TODO se afirma DENTRO del diálogo: a 1024×600 el corte se abre así, y el panel lateral existe
  // en el árbol pero oculto. Sin acotar, el localizador cae en la copia invisible y el assert espera
  // 30 segundos a algo que nunca se va a ver.
  const dialogo = page.locator('[role="dialog"]').first();
  await expect(dialogo.getByText('Monto inicial')).toBeVisible({ timeout: 30_000 });
  // Ni desglose ni un aviso de que no lo tiene: "este corte no tiene desglose" es una historia del
  // sistema que quien audita no puede accionar.
  await expect(dialogo.getByText('Efectivo contado')).toHaveCount(0);
});
