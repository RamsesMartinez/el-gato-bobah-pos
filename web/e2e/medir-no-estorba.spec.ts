import { test, expect, type Page } from '@playwright/test';

// LA MEDICIÓN NO SE METE EN EL CAMINO DEL OPERADOR (spec 017, US3 · SC-003).
//
// Esta es la promesa central de la feature y hasta aquí solo la comprobaba un humano leyendo el
// quickstart. Una promesa que solo se verifica a mano se rompe el día que nadie tiene tiempo de
// verificarla — y se rompe en silencio, porque lo que falla es la velocidad del mostrador, no un
// error en pantalla.
//
// Lo que se simula NO es la red caída: es la red del restaurante, que responde pero tarde. El
// endpoint de medición se bloquea a propósito mientras el resto de la aplicación sigue viva.

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

test('M1 · con la medición muerta, el POS se usa igual', async ({ page }) => {
  // El endpoint de uso nunca contesta. Es el peor caso: ni éxito ni error, la petición colgada.
  let intentos = 0;
  await page.route('**/api/v1/usage', async (route) => {
    intentos++;
    // Se cuelga: si el registrador esperara la respuesta, la pantalla se quedaría esperando con él.
    // Es el caso peor —ni éxito ni error— y el que más se parece al wifi de un restaurante.
    await new Promise((r) => setTimeout(r, 60_000));
    await route.abort().catch(() => {});
  });

  await entrar(page);

  // Navegar DENTRO de la aplicación, como lo hace el operador: el POS es una SPA y una recarga
  // completa por cada pantalla no es lo que ocurre en el mostrador.
  for (const nombre of ['Pedidos', 'Caja']) {
    const enlace = page.getByRole('link', { name: nombre }).or(page.getByRole('button', { name: nombre }));
    if (await enlace.first().isVisible().catch(() => false)) {
      await enlace.first().click();
      await page.waitForTimeout(500);
    }
  }

  // El lote sale por tiempo a los 10 segundos. Se espera a que salga: si no, este test mediría que
  // el registrador está apagado y lo llamaría éxito.
  await page.waitForTimeout(12_000);

  await page.goto('/pos');
  // El POS sigue respondiendo: el catálogo carga y la cuenta se puede abrir.
  await expect(page.getByRole('button', { name: 'Cuenta 1' })).toBeVisible({ timeout: 30_000 });

  // Y ni un aviso sobre la medición. El operador no tiene por qué enterarse de que existe.
  const avisos = await page.getByText(/uso|medici[óo]n|analytics/i).count();
  expect(avisos, 'la medición no se le menciona a quien opera').toBe(0);

  // Que el registrador HAYA intentado mandar algo es lo que hace válido el resto del test: sin un
  // solo intento, esto pasaría en verde con la feature apagada.
  expect(intentos, 'el registrador no intentó mandar nada: el test no probó lo que dice').toBeGreaterThan(0);
});

test('M2 · con la medición muerta, capturar tocando rápido responde igual', async ({ page }) => {
  // SC-004 de la 019: el escuchador de toques va en la fase de captura del documento, que es justo
  // donde un manejador mal escrito se traga el evento antes de que llegue al control. Con la red de
  // la medición colgada, cada toque además encola — y si encolar costara algo, se notaría aquí y no
  // en un test unitario con el DOM simulado.
  let intentos = 0;
  await page.route('**/api/v1/usage', async (route) => {
    intentos++;
    await new Promise((r) => setTimeout(r, 60_000));
    await route.abort().catch(() => {});
  });

  await entrar(page);

  // Tocar un producto. En este catálogo casi todos abren la hoja de modificadores, así que el
  // primer toque se comprueba por su EFECTO: si el escuchador cancelara o detuviera el evento, la
  // hoja no abriría y el mostrador se quedaría sin poder capturar.
  const producto = page.getByRole('button').filter({ hasText: /\$/ }).first();
  await producto.click({ timeout: 15_000 });
  const hoja = page.getByRole('dialog').first();
  await expect(hoja, 'el toque no llegó al producto: la medición se metió en el camino del dedo').toBeVisible({
    timeout: 15_000,
  });

  // Y ahora la ráfaga, DENTRO de la hoja: son los toques que la rejilla no cuenta —es una capa
  // encima— pero que tienen que seguir funcionando igual. Es el peor caso de los dos mundos.
  const opciones = hoja.getByRole('button').filter({ hasText: /\d|Sin/ });
  const cuantas = Math.min(await opciones.count(), 5);
  const arranque = Date.now();
  for (let i = 0; i < cuantas; i++) {
    await opciones.nth(i).click({ timeout: 5_000 }).catch(() => {});
  }
  const tardanza = Date.now() - arranque;
  if (cuantas > 0) {
    // El margen es amplio a propósito —el ambiente de pruebas es una VM chica— porque lo que este
    // número atrapa es un `await` en el camino del toque, que costaría segundos por toque.
    expect(tardanza, 'tocar se volvió lento con la medición colgada').toBeLessThan(cuantas * 3_000);
  }

  // La pantalla sigue viva después de la ráfaga: se cierra la hoja y el POS responde.
  await page.keyboard.press('Escape');
  await expect(page.getByRole('button', { name: 'Cuenta 1' })).toBeVisible({ timeout: 30_000 });

  await page.waitForTimeout(12_000);
  expect(intentos, 'el registrador no intentó mandar nada: el test no probó lo que dice').toBeGreaterThan(0);
});
