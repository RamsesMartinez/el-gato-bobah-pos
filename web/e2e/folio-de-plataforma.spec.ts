import { test, expect, type Page, type APIRequestContext } from '@playwright/test';

import { API, USUARIO, EMPRESA, PASSWORD } from './ambiente';

// EL FOLIO DE LA PLATAFORMA, A 1024×600 Y CONTRA EL SERVIDOR DE VERDAD (spec 014).
//
// Lo que solo se ve aquí y no en vitest: el ALTO que las cosas ocupan de verdad. Las medidas de
// Chakra son clases CSS y jsdom no las resuelve, así que allá un assert de píxeles pasa en verde
// con los controles chicos. Y el desacuerdo entre lo que la pantalla manda y lo que el servidor
// acepta solo aparece con los dos hablando.

// El login SIEMPRE entra por `/` y la ruta se pide DESPUÉS.
//
// El access token vive solo en memoria (para que un XSS no pueda leerlo), así que ir directo a
// `/ventas` sin sesión manda al login y, al entrar, la app aterriza en su ruta por defecto — el
// POS. El test medía entonces la pantalla equivocada y fallaba diciendo que faltaba un botón que
// sí existe. Es la misma trampa que ya documenta `pantallas.spec.ts`.
async function entrar(page: Page, ruta = '/') {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  const usuario = page.getByPlaceholder('usuario@empresa');
  if (await usuario.isVisible().catch(() => false)) {
    await usuario.fill(`${USUARIO}@${EMPRESA}`);
    await page.getByPlaceholder('Contraseña').fill(PASSWORD);
    await page.getByRole('button', { name: 'Entrar', exact: true }).click();
    await page.waitForURL(/\/(pos)?$/);
  }
  if (ruta !== '/') await page.goto(ruta);
  await page.waitForLoadState('networkidle');
}

async function token(request: APIRequestContext): Promise<string> {
  const r = await request.post(`${API}/auth/login`, {
    data: { username: USUARIO, slug: EMPRESA, password: PASSWORD },
  });
  expect(r.ok(), 'el login del ambiente de pruebas falló').toBeTruthy();
  return (await r.json()).accessToken;
}

// renglonesDelMosaico cuenta los renglones COMPLETOS de producto que el operador ve.
//
// El método es el del documento de presupuesto y no se re-deriva: el mosaico es el único div con
// display:grid, más de una columna y más de cuatro hijos; el piso real es min(contenedor.bottom,
// 600) porque lo que queda debajo de 600 no lo ve nadie, tenga o no scroll.
async function renglonesDelMosaico(page: Page): Promise<{ renglones: number; sobrante: number }> {
  return page.evaluate(() => {
    const grids = [...document.querySelectorAll('div')].filter((d) => {
      const s = getComputedStyle(d);
      return s.display === 'grid'
        && s.gridTemplateColumns.split(' ').length > 1
        && d.children.length > 4;
    });
    const grid = grids[0];
    if (!grid) return { renglones: 0, sobrante: 0 };
    let cont: HTMLElement | null = grid.parentElement;
    while (cont && getComputedStyle(cont).overflowY !== 'auto') cont = cont.parentElement;
    const caja = (cont ?? grid).getBoundingClientRect();
    const ficha = grid.children[0].getBoundingClientRect();
    const gap = parseFloat(getComputedStyle(grid).rowGap || '0');
    const paso = ficha.height + gap;
    const visible = Math.min(caja.bottom, 600) - caja.top;
    const renglones = Math.floor((visible + gap) / paso);
    return { renglones, sobrante: Math.round(visible - (renglones * paso - gap)) };
  });
}

test.describe('Y · el folio de la plataforma en el POS', () => {
  test('Y6+Y9 · el campo no existe en Mostrador, y con plataforma activa el mosaico conserva sus renglones', async ({ page }) => {
    await entrar(page);
    await expect(page.getByRole('button', { name: 'Cuenta 1' })).toBeVisible({ timeout: 30_000 });

    // El aviso de turno viejo desplaza el shell 53 px sin restárselos de su alto: se quita para
    // medir el estado que el documento de presupuesto llama "sin el aviso", que es el más apretado.
    const ahoraNo = page.getByRole('button', { name: /Ahora no/i });
    if (await ahoraNo.isVisible().catch(() => false)) await ahoraNo.click();

    // Una subcategoría y no "Top": esa pestaña está topada a `topCount` fichas, que es una
    // preferencia del usuario y no la pantalla.
    const primeraCategoria = page.locator('[role="tab"], button').filter({ hasText: /^(Crepas|Waffles|Bebidas|Postres)/ }).first();
    if (await primeraCategoria.isVisible().catch(() => false)) await primeraCategoria.click();
    await page.waitForTimeout(500);

    const enMostrador = await renglonesDelMosaico(page);
    expect(enMostrador.renglones, 'no se encontró el mosaico').toBeGreaterThan(0);

    // En Mostrador el campo NO EXISTE en el árbol, no "existe oculto".
    expect(await page.getByLabel(/^Folio de /).count()).toBe(0);

    const uber = page.getByRole('button', { name: /Uber Eats/ });
    test.skip(!(await uber.isVisible().catch(() => false)), 'este negocio no tiene plataformas configuradas');
    await uber.click();
    await page.waitForTimeout(500);

    // Aparece, nombra la plataforma y mide al menos 44 px.
    const campo = page.getByLabel('Folio de Uber Eats').first();
    await expect(campo).toBeVisible();
    const caja = await campo.boundingBox();
    expect(Math.round(caja?.height ?? 0), 'el campo del folio no llega al piso táctil').toBeGreaterThanOrEqual(44);

    const conPlataforma = await renglonesDelMosaico(page);
    // SC-007: al menos lo de hoy menos un renglón, y el número medido se DECLARA aquí y en
    // docs/presupuesto-de-pantalla-1024x600.md.
    console.log(`[medido 1024×600] mosaico en mostrador: ${enMostrador.renglones} renglones ` +
      `(${enMostrando(enMostrador.sobrante)}); con plataforma y campo de folio: ` +
      `${conPlataforma.renglones} renglones (${enMostrando(conPlataforma.sobrante)})`);
    expect(conPlataforma.renglones,
      `con plataforma activa quedan ${conPlataforma.renglones} renglones y en mostrador ${enMostrador.renglones}: ` +
      'SC-007 permite perder uno, no dos').toBeGreaterThanOrEqual(enMostrador.renglones - 1);
  });

  test('Y7+Y8 · mandar sin folio pide el dato, y la salida sigue visible con el teclado abierto', async ({ page }) => {
    await entrar(page);
    await expect(page.getByRole('button', { name: 'Cuenta 1' })).toBeVisible({ timeout: 30_000 });
    const ahoraNo = page.getByRole('button', { name: /Ahora no/i });
    if (await ahoraNo.isVisible().catch(() => false)) await ahoraNo.click();

    const uber = page.getByRole('button', { name: /Uber Eats/ });
    test.skip(!(await uber.isVisible().catch(() => false)), 'este negocio no tiene plataformas configuradas');
    await uber.click();

    await page.getByText('Dedos de Queso Pza').first().click();
    const confirmar = page.getByRole('button', { name: /^(Agregar|Confirmar)/ });
    if (await confirmar.isVisible().catch(() => false)) await confirmar.click();

    // Con el campo VACÍO, mandar se interpone.
    const pildora = page.getByRole('button', { name: /art ·/ });
    if (await pildora.isVisible().catch(() => false)) await pildora.click();
    await page.getByRole('button', { name: 'COBRAR' }).click();

    const hoja = page.getByText(/¿Con qué folio llegó de Uber Eats\?/);
    await expect(hoja, 'mandar con el campo vacío no pidió el folio').toBeVisible({ timeout: 15_000 });

    // UNA sola salida.
    await expect(page.getByRole('button', { name: /sin folio/i })).toHaveCount(1);

    // EL TECLADO. Chromium headless no abre el teclado del sistema, así que se SIMULA lo que hace:
    // encoger la ventana visual ~250 px. Con el footer dentro del scroll, el botón de escape queda
    // fuera y la salida pasa de un toque a dos.
    await page.setViewportSize({ width: 1024, height: 350 });
    await page.waitForTimeout(300);
    const salida = page.getByRole('button', { name: /sin folio/i });
    const cajaSalida = await salida.boundingBox();
    expect(cajaSalida, 'la salida desapareció al encoger la ventana').not.toBeNull();
    expect(cajaSalida!.y + cajaSalida!.height,
      'con el teclado abierto la salida queda fuera de la pantalla: SC-003 pasa de un toque a dos')
      .toBeLessThanOrEqual(350);
    await page.setViewportSize({ width: 1024, height: 600 });

    // Tomar la salida manda el pedido sin folio. Queda en la barra y el teardown lo cobra.
    await salida.click();
    await expect(page.getByText(/¿Con qué folio llegó/)).toHaveCount(0, { timeout: 15_000 });
  });
});

test.describe('Y · Ventas: buscar, filtrar y liquidar', () => {
  test('Y12 · pegar el folio del documento encuentra el pedido en un paso', async ({ request }) => {
    const jwt = await token(request);
    // Un folio que nadie capturó devuelve la lista VACÍA y no un error: es la respuesta a "este
    // renglón del documento todavía no está registrado".
    const r = await request.get(`${API}/sales?preset=mes&folio=NO-EXISTE-${Date.now()}`, {
      headers: { Authorization: `Bearer ${jwt}` },
    });
    expect(r.status(), 'buscar un folio no capturado no puede ser un error').toBe(200);
    expect((await r.json()).items).toHaveLength(0);
  });

  test('Y10 · un filtro de pendientes desconocido se rechaza, no cae a "todos"', async ({ request }) => {
    const jwt = await token(request);
    const r = await request.get(`${API}/sales?preset=mes&folioPlataforma=pendientes`, {
      headers: { Authorization: `Bearer ${jwt}` },
    });
    expect(r.status(), 'un filtro desconocido cayó a "todos" y la pantalla mostraría un conjunto que nadie pidió').toBe(400);
  });

  test('Y11 · la lista y el resumen de pendientes describen el mismo conjunto', async ({ request }) => {
    const jwt = await token(request);
    const h = { Authorization: `Bearer ${jwt}` };
    const lista = await (await request.get(`${API}/sales?preset=mes&folioPlataforma=pendiente&pageSize=100`, { headers: h })).json();
    const resumen = await (await request.get(`${API}/sales/summary?preset=mes&folioPlataforma=pendiente`, { headers: h })).json();
    // Las canceladas y reembolsadas no entran al `count` del resumen, así que se comparan contra la
    // lista descontándolas — es la misma regla que aplica la pantalla.
    const vivas = (lista.items as Array<{ status: string }>).filter(
      (v) => v.status !== 'cancelada' && v.status !== 'reembolsada').length;
    expect(resumen.count,
      `la lista trae ${vivas} ventas vivas y el resumen dice ${resumen.count}: una de las cinco ` +
      'consultas se quedó sin el filtro').toBe(vivas);
  });

  test('Y14+Y18+F65 · la pantalla de Ventas a 1024×600', async ({ page }) => {
    await entrar(page, '/ventas');
    await expect(page.getByRole('button', { name: /Pendientes de folio/i })).toBeVisible({ timeout: 30_000 });

    // El buscador y el toggle miden al menos 44 px.
    for (const nombre of ['Buscar folio de la plataforma']) {
      const caja = await page.getByLabel(nombre).boundingBox();
      expect(Math.round(caja?.height ?? 0), `${nombre} no llega al piso táctil`).toBeGreaterThanOrEqual(44);
    }
    const toggle = await page.getByRole('button', { name: /Pendientes de folio/i }).boundingBox();
    expect(Math.round(toggle?.height ?? 0)).toBeGreaterThanOrEqual(44);

    // T065: cuántos renglones de tabla quedan a la vista. Es la PRIMERA medición de esta pantalla:
    // el documento de presupuesto solo cubría el POS.
    const renglones = await page.evaluate(() => {
      const filas = [...document.querySelectorAll('tbody tr')];
      return filas.filter((f) => f.getBoundingClientRect().bottom <= 600).length;
    });
    console.log(`[medido 1024×600] Ventas: ${renglones} renglones de tabla visibles con el ` +
      'buscador, el toggle y los tiles puestos');
    expect(renglones, 'la pantalla de Ventas se quedó sin renglones de tabla a la vista').toBeGreaterThan(0);

    // La página NO scrollea horizontalmente: el folio va truncado dentro de su celda, no como
    // columna nueva.
    const desborde = await page.evaluate(() =>
      document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(desborde, 'la página se desborda a lo ancho: el folio se pintó como columna').toBeLessThanOrEqual(0);
  });
});

function enMostrando(sobrante: number): string {
  return `${sobrante} px de sobra`;
}
