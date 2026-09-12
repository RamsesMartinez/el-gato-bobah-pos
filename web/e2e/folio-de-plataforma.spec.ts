import { test, expect, type Page } from '@playwright/test';

import { API, USUARIO, EMPRESA, PASSWORD, tokenDeRequest } from './ambiente';

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

    await ponerUnProductoEnLaCuenta(page);
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

// UNA CUENTA GUARDADA POR LA VERSIÓN ANTERIOR NO PUEDE DEJAR EL POS EN BLANCO.
//
// Es el defecto que de verdad se vio en dev, sobre Chrome: `platformOrderRef` nació con esta
// feature, las cuentas ya guardadas en la tableta no lo traían, y `FolioPlataformaSheet` —que vive
// SIEMPRE montada— hace `useState(valorInicial).trim()` en su primer render. El POS entero se caía
// antes de pintar: pantalla en blanco al entrar.
//
// SEMBRAR EL CARRITO VIEJO NO BASTA, y por eso este caso pasaba en verde contra el build roto:
// Playwright arranca con un perfil limpio, sin la marca `sesion.ultimaEmpresa`, y `hayQueLimpiar`
// trata un dispositivo sin marca como cambio de empresa — el login llama a `descartarTodo()` y el
// carrito sembrado se va a la basura antes de que el POS renderice. La tableta de un operador SÍ
// tiene la marca, así que ahí la cuenta vieja sobrevive al login y es la que truena. Hay que
// sembrar las dos cosas para reproducir la tableta de verdad.

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

test('Y20 · una cuenta guardada antes de esta feature no deja el POS en blanco', async ({ page, request }) => {
  const r = await request.post(`${API}/auth/login`, {
    data: { username: USUARIO, slug: EMPRESA, password: PASSWORD },
  });
  expect(r.ok(), 'el login del ambiente de pruebas falló').toBeTruthy();
  const companyId: number = (await r.json()).user.companyId;

  const errores: string[] = [];
  page.on('pageerror', (e) => errores.push(e.message.slice(0, 200)));

  await page.addInitScript((empresa: number) => {
    // La marca de empresa: sin ella el login limpia el carrito y el defecto no se reproduce.
    localStorage.setItem('sesion.ultimaEmpresa', String(empresa));
    localStorage.setItem('egb:ticket:v2', JSON.stringify({
      state: {
        tabs: [{
          id: 'vieja-1', num: 1, folioName: 'Tigre', lines: [], envio: '',
          serviceType: 'mostrador', customerName: '', platformId: null,
          // sin platformOrderRef, tal como se guardaba antes
        }],
        activeId: 'vieja-1', seq: 2,
      },
      version: 0,
    }));
  }, companyId);

  await entrar(page);

  // La cuenta VIEJA es la que tiene que seguir ahí: si el POS renderizó porque el login la tiró,
  // este caso volvería a pasar en verde con el defecto puesto.
  await expect(page.getByRole('button', { name: /Tigre/ }),
    'el carrito sembrado no sobrevivió al login: el caso no está reproduciendo la tableta de un operador')
    .toBeVisible({ timeout: 30_000 });

  // Y elegir plataforma —lo primero que toca el campo nuevo— tampoco truena.
  const uber = page.getByRole('button', { name: /Uber Eats/ });
  if (await uber.isVisible().catch(() => false)) {
    await uber.click();
    await expect(page.getByLabel('Folio de Uber Eats').first()).toBeVisible();
  }
  expect(errores, `el render tiró: ${errores.join(' | ')}`).toHaveLength(0);
});

test.describe('Y · Ventas: buscar, filtrar y liquidar', () => {
  test('Y12 · pegar el folio del documento encuentra el pedido en un paso', async ({ request }) => {
    const jwt = await tokenDeRequest(request);
    // Un folio que nadie capturó devuelve la lista VACÍA y no un error: es la respuesta a "este
    // renglón del documento todavía no está registrado".
    const r = await request.get(`${API}/sales?preset=mes&folio=NO-EXISTE-${Date.now()}`, {
      headers: { Authorization: `Bearer ${jwt}` },
    });
    expect(r.status(), 'buscar un folio no capturado no puede ser un error').toBe(200);
    expect((await r.json()).items).toHaveLength(0);
  });

  test('Y10 · un filtro de pendientes desconocido se rechaza, no cae a "todos"', async ({ request }) => {
    const jwt = await tokenDeRequest(request);
    const r = await request.get(`${API}/sales?preset=mes&folioPlataforma=pendientes`, {
      headers: { Authorization: `Bearer ${jwt}` },
    });
    expect(r.status(), 'un filtro desconocido cayó a "todos" y la pantalla mostraría un conjunto que nadie pidió').toBe(400);
  });

  test('Y11 · la lista y el resumen de pendientes describen el mismo conjunto', async ({ request }) => {
    const jwt = await tokenDeRequest(request);
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

  test('Y14+F65 · la pantalla de Ventas a 1024×600', async ({ page }) => {
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

    // El alto de la fila CON folio contra una sin él. La celda "Tipo" pasa de una línea a dos, y
    // la pregunta es si eso empuja el renglón — la columna "Folio" ya es de dos líneas en casi todo
    // pedido, así que no debería, pero eso es cálculo y esto es medición.
    const altos = await page.evaluate(() => {
      const filas = [...document.querySelectorAll('tbody tr')];
      const conFolio: number[] = [];
      const sinFolio: number[] = [];
      for (const f of filas) {
        const celdas = [...f.querySelectorAll('td')];
        const tipo = celdas[3];
        const dosLineas = (tipo?.querySelectorAll('p, div') ?? []).length > 1;
        (dosLineas ? conFolio : sinFolio).push(Math.round(f.getBoundingClientRect().height));
      }
      return { conFolio, sinFolio };
    });
    if (altos.conFolio.length === 0) {
      // Se DECLARA en vez de pasar en verde sin haber medido: el mes de pruebas no trajo ningún
      // pedido con folio capturado, así que este caso no se ejerció en esta corrida.
      console.log('[medido 1024×600] Y14 · el mes no trae pedidos con folio capturado: el alto de ' +
        'la fila con folio NO se midió en esta corrida');
    } else {
      const max = Math.max(...altos.conFolio);
      const base = altos.sinFolio.length ? Math.max(...altos.sinFolio) : max;
      console.log(`[medido 1024×600] Y14 · fila con folio: ${max}px · sin folio: ${base}px`);
      expect(max, 'la fila con folio de plataforma creció más de un renglón sobre las demás')
        .toBeLessThanOrEqual(base + 20);
    }

    // La página NO scrollea horizontalmente: el folio va truncado dentro de su celda, no como
    // columna nueva.
    const desborde = await page.evaluate(() =>
      document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(desborde, 'la página se desborda a lo ancho: el folio se pintó como columna').toBeLessThanOrEqual(0);
  });
});

test.describe('Y18 · la hoja de la liquidación a 1024×600', () => {
  test('los nueve campos caben, y el botón de guardar sigue visible con el teclado abierto', async ({ page }) => {
    await entrar(page, '/ventas');
    await page.getByRole('button', { name: 'Mes', exact: true }).click();
    await page.waitForLoadState('networkidle');

    // Un renglón de plataforma: es el único que tiene liquidación. Si el mes no trae ninguno, no
    // hay nada que medir y se dice, en vez de pasar en verde sin haber abierto la hoja.
    const renglon = page.locator('tbody tr').filter({ hasText: /Uber Eats|Didi|Rappi/ }).first();
    test.skip(!(await renglon.isVisible().catch(() => false)),
      'el mes no tiene pedidos de plataforma: no hay liquidación que medir');
    await renglon.click();

    await page.getByRole('button', { name: /^(Registrar|Corregir)$/ }).click();
    await expect(page.getByLabel('Venta que reporta')).toBeVisible({ timeout: 15_000 });

    // Los nueve campos del documento existen y todos llegan al piso táctil.
    const campos = ['Venta que reporta', 'Comisión', 'Tasa de comisión (%)', 'Retenciones',
      'Descuento total', 'Lo puso la plataforma', 'Depositado (neto)',
      'Referencia del depósito', 'Documento'];
    for (const c of campos) {
      // exact: 'Comisión' es substring de 'Tasa de comisión (%)' y sin esto el locator resuelve dos.
      const caja = await page.getByLabel(c, { exact: true }).boundingBox();
      expect(caja, `falta el campo ${c}`).not.toBeNull();
      expect(Math.round(caja!.height), `${c} no llega al piso táctil`).toBeGreaterThanOrEqual(44);
    }

    // La hoja entera cabe en la tableta.
    const hoja = await page.locator('[role="dialog"]').last().boundingBox();
    expect(Math.round(hoja?.height ?? 0), 'la hoja de liquidación no cabe en 600 px').toBeLessThanOrEqual(600);

    // EL TECLADO. Se simula encogiendo la ventana ~250 px, que es lo que se lleva el numérico.
    // Con el footer dentro del scroll, guardar quedaría debajo y la captura se volvería imposible
    // sin cerrar el teclado primero.
    await page.setViewportSize({ width: 1024, height: 350 });
    await page.waitForTimeout(300);
    const guardar = await page.getByRole('button', { name: /Guardar liquidación/i }).boundingBox();
    expect(guardar, 'el botón de guardar desapareció al encoger la ventana').not.toBeNull();
    expect(guardar!.y + guardar!.height,
      'con el teclado abierto el botón de guardar queda fuera de la pantalla')
      .toBeLessThanOrEqual(350);
    await page.setViewportSize({ width: 1024, height: 600 });

    // Se cierra SIN guardar: este test mide la pantalla, no captura dinero en un ambiente
    // compartido.
    await page.getByRole('button', { name: 'Cancelar' }).click();
  });
});

function enMostrando(sobrante: number): string {
  return `${sobrante} px de sobra`;
}
