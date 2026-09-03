import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
import { API, EMPRESA, PASSWORD, USUARIO } from './ambiente';

// LA MATRIZ DE PANTALLAS, EXTREMO A EXTREMO. Ver docs/matriz-de-pantallas.md.
//
// Lo que estas pruebas atrapan y las de vitest no: que la pantalla y el servidor estén hablando del
// MISMO periodo. Con el backend simulado, la pantalla y su mock coinciden por construcción — que es
// justo la coincidencia que aquí hay que poner en duda.
//
// NO CREAN PEDIDOS. Es a propósito: el ambiente lo comparte una persona y lo que estas pruebas
// miden —rangos, cotas, cuadres, alto de pantalla— se mide sobre lo que ya hay. Lo que sí crea
// pedidos vive en dinero.spec.ts, que además los cobra.

async function token(request: APIRequestContext): Promise<string> {
  const r = await request.post(`${API}/auth/login`, {
    data: { username: USUARIO, slug: EMPRESA, password: PASSWORD },
  });
  expect(r.ok(), 'el login del ambiente de pruebas falló').toBeTruthy();
  return (await r.json()).accessToken;
}

async function entrar(page: Page, jwt: string, ruta: string) {
  await page.addInitScript((t) => {
    window.localStorage.setItem('pos.session', JSON.stringify({ state: { token: t }, version: 0 }));
  }, jwt);
  await page.goto(ruta);
}

// alturaDe mide lo que un elemento le quita al presupuesto de 600 px. Devuelve 0 si no existe.
async function alturaDe(page: Page, selector: string): Promise<number> {
  const caja = await page.locator(selector).first().boundingBox();
  return caja?.height ?? 0;
}

test.describe('R — el rango de fechas contra el servidor real', () => {
  test('R1 · un preset inventado se rechaza, no cae a hoy', async ({ request }) => {
    const jwt = await token(request);
    const auth = { Authorization: `Bearer ${jwt}` };
    for (const ruta of ['/sales?preset=el-mes-pasado-pero-solo-martes', '/reports/sales?preset=inventado']) {
      const r = await request.get(`${API}${ruta}`, { headers: auth });
      expect(r.status(), `${ruta} debería rechazar el preset, no contestar otro periodo`)
        .toBeGreaterThanOrEqual(400);
      expect(r.status()).toBeLessThan(500);
    }
  });

  // Invertido devuelve CERO filas sin error si nadie lo rechaza, y quien lo lee cree que no vendió.
  test('R2 · un rango invertido se rechaza', async ({ request }) => {
    const jwt = await token(request);
    const auth = { Authorization: `Bearer ${jwt}` };
    const q = 'preset=rango&from=2026-08-31&to=2026-08-01';
    for (const base of ['/sales', '/reports/sales', '/reports/tips', '/reports/margins']) {
      const r = await request.get(`${API}${base}?${q}`, { headers: auth });
      expect(r.status(), `${base} aceptó un rango invertido`).toBeGreaterThanOrEqual(400);
      expect(r.status()).toBeLessThan(500);
    }
  });

  // Sin cota, un "del 2020 a hoy" escanea sin límite en el gigabyte de RAM del VPS.
  test('R3 · un rango de años se rechaza en las cuatro rutas', async ({ request }) => {
    const jwt = await token(request);
    const auth = { Authorization: `Bearer ${jwt}` };
    const q = 'preset=rango&from=2020-01-01&to=2026-12-31';
    for (const base of ['/sales', '/sales/summary', '/reports/sales', '/reports/margins']) {
      const r = await request.get(`${API}${base}?${q}`, { headers: auth });
      expect(r.status(), `${base} aceptó un rango de años`).toBeGreaterThanOrEqual(400);
      expect(r.status()).toBeLessThan(500);
    }
  });

  // Una fecha que el preset no va a usar se descartaba en silencio: la respuesta era HOY con la
  // pantalla viéndose perfecta.
  test('R5 · fechas con un preset que no las usa se rechazan', async ({ request }) => {
    const jwt = await token(request);
    const auth = { Authorization: `Bearer ${jwt}` };
    const r = await request.get(`${API}/sales?preset=hoy&from=2026-01-01&to=2026-01-31`, { headers: auth });
    expect(r.status(), 'se aceptaron unas fechas que el preset iba a descartar')
      .toBeGreaterThanOrEqual(400);
    expect(r.status()).toBeLessThan(500);
  });

  test('R8 · una fecha malformada se rechaza, no cae al default', async ({ request }) => {
    const jwt = await token(request);
    const auth = { Authorization: `Bearer ${jwt}` };
    for (const mala of ['31/08/2026', '2026-13-45', 'ayer', '2026-8-1']) {
      const r = await request.get(
        `${API}/reports/sales?preset=rango&from=${encodeURIComponent(mala)}&to=2026-09-01`,
        { headers: auth },
      );
      expect(r.status(), `la fecha ${mala} no se rechazó`).toBeGreaterThanOrEqual(400);
      expect(r.status()).toBeLessThan(500);
    }
  });
});

test.describe('Q — los reportes responden un solo periodo', () => {
  // EL CUADRE QUE NINGÚN UNITARIO PUEDE HACER: sobre los datos reales del ambiente, la suma de los
  // medios de pago no puede pasarse de la venta del periodo. Si se pasa, alguna de las dos tablas
  // está contestando otro rango o incluyendo ventas que la otra excluye.
  //
  // No se exige igualdad: una venta mandada a cocina y todavía sin cobrar suma a la venta del día y
  // no a ningún método, así que lo cobrado va POR DEBAJO de lo vendido. Lo que no puede pasar es lo
  // contrario — ahí es donde vivía el defecto.
  test('Q1 · lo cobrado por método nunca excede la venta del periodo', async ({ request }) => {
    const jwt = await token(request);
    const auth = { Authorization: `Bearer ${jwt}` };
    for (const preset of ['30d', 'semana', 'mes']) {
      const r = await request.get(`${API}/reports/sales?preset=${preset}`, { headers: auth });
      expect(r.ok(), `/reports/sales?preset=${preset}`).toBeTruthy();
      const { range, byDay, byMethod } = await r.json();

      expect(range?.from, 'la respuesta no dice qué periodo consultó').toBeTruthy();
      expect(range?.to).toBeTruthy();

      const vendido = byDay.reduce((s: number, d: { revenue: string }) => s + Number(d.revenue), 0);
      const cobrado = byMethod.reduce((s: number, m: { total: string }) => s + Number(m.total), 0);
      expect(cobrado, `en ${preset} (${range.from}..${range.to}) los métodos suman ${cobrado} `
        + `sobre una venta de ${vendido}: una de las dos tablas mira otro periodo`)
        .toBeLessThanOrEqual(vendido + 0.011);
    }
  });

  // Los tres reportes responden EL MISMO rango. Es lo que impide que la pantalla pinte dos periodos
  // uno junto a otro sin nada que lo delate.
  test('Q4 · los tres reportes devuelven el mismo rango', async ({ request }) => {
    const jwt = await token(request);
    const auth = { Authorization: `Bearer ${jwt}` };
    const q = 'preset=rango&from=2026-08-01&to=2026-08-31';
    const rangos: Record<string, string> = {};
    for (const base of ['/reports/sales', '/reports/tips', '/reports/margins']) {
      const r = await request.get(`${API}${base}?${q}`, { headers: auth });
      expect(r.ok(), `${base} con rango libre`).toBeTruthy();
      const { range } = await r.json();
      rangos[base] = `${range.from}..${range.to}`;
    }
    const distintos = new Set(Object.values(rangos));
    expect(distintos.size, `los reportes contestaron periodos distintos: ${JSON.stringify(rangos)}`)
      .toBe(1);
    expect([...distintos][0]).toBe('2026-08-01..2026-08-31');
  });

  // "30d" son TREINTA días contando hoy. El handler restaba 30 al día de fin, que son treinta y uno:
  // la diferencia no se ve, y por eso muerde al comparar dos periodos "de 30 días".
  test('Q6 · el preset de 30 días mide treinta días', async ({ request }) => {
    const jwt = await token(request);
    const r = await request.get(`${API}/reports/sales?preset=30d`, {
      headers: { Authorization: `Bearer ${jwt}` },
    });
    const { range } = await r.json();
    const dias = Math.round(
      (Date.parse(`${range.to}T00:00:00Z`) - Date.parse(`${range.from}T00:00:00Z`)) / 86_400_000,
    ) + 1;
    expect(dias, `el periodo ${range.from}..${range.to} mide ${dias} días`).toBe(30);
  });
});

test.describe('F — el control de rango en la tableta', () => {
  test('F8 · Ventas con el rango abierto deja renglones de tabla a la vista', async ({ page, request }) => {
    const jwt = await token(request);
    await entrar(page, jwt, '/ventas');

    await page.getByRole('button', { name: 'Rango' }).click();
    await expect(page.getByLabel('Desde')).toBeVisible();

    // El presupuesto es 600 px de alto. Lo que gastan el encabezado, los chips, las fechas, las
    // tarjetas de resumen y los filtros es alto que la tabla ya no tiene: si no quedan al menos dos
    // renglones a la vista, el filtro se comió la pantalla que vino a filtrar.
    const tabla = await page.locator('table').first().boundingBox();
    expect(tabla, 'no se encontró la tabla de ventas').not.toBeNull();
    const visible = 600 - (tabla?.y ?? 600);
    expect(visible, `a la tabla le quedan ${Math.round(visible)} px de los 600 de la tableta`)
      .toBeGreaterThan(120);
  });

  // 44 px es el mínimo con el que un dedo acierta a la primera. Por debajo el operador toca dos
  // veces y la segunda cae en otra cosa.
  test('F9 · los controles del rango miden al menos 44 px', async ({ page, request }) => {
    const jwt = await token(request);
    await entrar(page, jwt, '/ventas');

    for (const nombre of ['Hoy', 'Ayer', 'Semana', 'Mes', 'Rango']) {
      const caja = await page.getByRole('button', { name: nombre }).boundingBox();
      expect(caja?.height ?? 0, `el chip "${nombre}" mide ${caja?.height} px`).toBeGreaterThanOrEqual(44);
    }
    await page.getByRole('button', { name: 'Rango' }).click();
    for (const campo of ['Desde', 'Hasta']) {
      const caja = await page.getByLabel(campo).boundingBox();
      expect(caja?.height ?? 0, `el campo "${campo}" mide ${caja?.height} px`).toBeGreaterThanOrEqual(44);
    }
  });

  // Un rango a medias no consulta: la pantalla conserva el periodo anterior —que su encabezado sigue
  // nombrando— y dice qué falta. Sin esto se manda media fecha y el servidor contesta un 400 que el
  // operador lee como "se descompuso".
  test('F2 · con una sola fecha no se consulta y se dice qué falta', async ({ page, request }) => {
    const jwt = await token(request);
    await entrar(page, jwt, '/ventas');
    await page.getByRole('button', { name: 'Rango' }).click();

    let peticiones = 0;
    page.on('request', (r) => { if (r.url().includes('/sales?')) peticiones += 1; });
    await page.getByLabel('Desde').fill('2026-08-01');

    await expect(page.getByText(/Elige las dos fechas/)).toBeVisible();
    expect(peticiones, 'se consultó con media fecha').toBe(0);
  });

  test('F3 · un rango invertido se avisa antes de ir al servidor', async ({ page, request }) => {
    const jwt = await token(request);
    await entrar(page, jwt, '/ventas');
    await page.getByRole('button', { name: 'Rango' }).click();
    await page.getByLabel('Desde').fill('2026-08-31');
    await page.getByLabel('Hasta').fill('2026-08-01');

    await expect(page.getByText(/La fecha de inicio va antes que la de fin/)).toBeVisible();
  });

  // El encabezado dice el periodo que el SERVIDOR consultó. Una cifra sin su periodo no se puede
  // auditar; una con el periodo equivocado se audita mal.
  test('F10 · Reportes imprime el periodo que consultó', async ({ page, request }) => {
    const jwt = await token(request);
    await entrar(page, jwt, '/reportes');

    await page.getByRole('button', { name: 'Rango' }).click();
    await page.getByLabel('Desde').fill('2026-08-01');
    await page.getByLabel('Hasta').fill('2026-08-31');

    await expect(page.getByText('2026-08-01 al 2026-08-31')).toBeVisible();
    await expect(page.getByText(/últimos 30 días/i)).toHaveCount(0);
  });

  test('F11 · Reportes cabe en la tableta con el rango abierto', async ({ page, request }) => {
    const jwt = await token(request);
    await entrar(page, jwt, '/reportes');
    await page.getByRole('button', { name: 'Rango' }).click();
    await expect(page.getByLabel('Desde')).toBeVisible();

    // Nada del filtro puede empujar el contenido fuera de la pantalla horizontalmente: el cuerpo no
    // se desplaza de lado en una tableta, se desplazan las tablas dentro de su caja.
    const ancho = await page.evaluate(() => document.body.scrollWidth);
    expect(ancho, `el cuerpo mide ${ancho} px de ancho sobre una pantalla de 1024`)
      .toBeLessThanOrEqual(1024);

    // Y el bloque de filtros no puede comerse la mitad del alto.
    const gastado = await alturaDe(page, 'header') + await alturaDe(page, 'h1, h2');
    expect(gastado).toBeLessThan(300);
  });
});
