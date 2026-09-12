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
