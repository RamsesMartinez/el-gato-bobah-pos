import { test, expect } from '@playwright/test';
import { API, tokenDeRequest } from './ambiente';

// LOS CASOS T Y U DE LA MATRIZ, CONTRA EL SERVIDOR REAL. Ver docs/matriz-de-pantallas.md.
//
// Lo que estas pruebas atrapan y las de Go no: que la venta, la pantalla de Ventas y el corte estén
// hablando del MISMO día sobre datos que nadie preparó para ellas. El defecto que las trajo vivía
// justo ahí — cada pieza contestaba algo coherente por su cuenta y entre las tres había cinco días
// de diferencia.
//
// NO CREAN PEDIDOS. El ambiente lo comparte una persona y estas mediciones se hacen sobre lo que ya
// hay; lo que sí crea pedidos vive en dinero.spec.ts, que además los cobra.


test.describe('T — la fecha la da el reloj', () => {
  // EL DEFECTO REPORTADO, medido donde ocurrió.
  //
  // Ninguna venta puede estar archivada en un día distinto de aquel en que se abrió. Con la
  // herencia vieja, un turno olvidado archivaba semanas enteras bajo su fecha de apertura.
  test('T1 · ninguna venta quedó archivada en un día que no es el suyo', async ({ request }) => {
    const jwt = await tokenDeRequest(request);
    // `to` es HOY en la zona del local, nunca una fecha futura: el servidor rechaza el rango que
    // no ha pasado, y con razón — pedir hasta diciembre devolvía 400 y esta prueba culpaba al
    // producto de su propio error.
    const hoy = new Date().toLocaleDateString('en-CA', { timeZone: 'America/Mexico_City' });
    const r = await request.get(`${API}/sales?preset=rango&from=2026-07-01&to=${hoy}&limit=200`, {
      headers: { Authorization: `Bearer ${jwt}` },
    });
    expect(r.ok(), `/sales respondió ${r.status()}`).toBeTruthy();
    const { items } = await r.json();
    expect(Array.isArray(items)).toBeTruthy();

    const corridas = (items as Array<{ id: number; date: string; openedAt: string }>).filter((v) => {
      // El día local del negocio (America/Mexico_City) del instante en que se abrió el pedido.
      const local = new Date(v.openedAt).toLocaleDateString('en-CA', { timeZone: 'America/Mexico_City' });
      return local !== v.date;
    });
    expect(
      corridas.map((v) => `#${v.id} archivada el ${v.date} y abierta el ${v.openedAt}`),
      'hay ventas archivadas en un día distinto del que ocurrieron: la fecha se está heredando del turno',
    ).toEqual([]);
  });

  // El folio es único DENTRO DEL TURNO. Es el alcance nuevo, y el índice que lo vigila se movió con
  // él: si el servicio y el esquema no coincidieran, la venta se caería con un 23505 al cobrar.
  test('T2 · dentro de un mismo corte no hay dos folios iguales', async ({ request }) => {
    const jwt = await tokenDeRequest(request);
    const auth = { Authorization: `Bearer ${jwt}` };
    const h = await request.get(`${API}/cash-sessions?limit=10`, { headers: auth });
    expect(h.ok(), `/cash-sessions respondió ${h.status()}`).toBeTruthy();
    const cortes = (await h.json()).items as Array<{ id: number }>;
    expect(cortes.length, 'el ambiente no tiene cortes que revisar').toBeGreaterThan(0);

    for (const c of cortes.slice(0, 5)) {
      const d = await request.get(`${API}/cash-sessions/${c.id}`, { headers: auth });
      expect(d.ok(), `el detalle del corte ${c.id} respondió ${d.status()}`).toBeTruthy();
      const det = await d.json();
      const folios = (det.sales ?? []).map((v: { dailyNumber: number }) => v.dailyNumber);
      expect(new Set(folios).size, `el corte ${c.id} tiene folios repetidos`).toBe(folios.length);
    }
  });
});

test.describe('U — las ventas de un corte', () => {
  // El total del corte no puede incluir lo que no dejó ingreso. Es el principio de dinero: cada peso
  // se clasifica una sola vez, y una cancelada nunca entró al cajón.
  test('U1 · el total del corte excluye canceladas y reembolsadas', async ({ request }) => {
    const jwt = await tokenDeRequest(request);
    const auth = { Authorization: `Bearer ${jwt}` };
    // TODOS los que traiga la página, no los cinco primeros.
    //
    // Los primeros cinco eran de la caja fuerte —que no vende— porque esta misma suite la abre y
    // la cierra para medir la hoja del contador, y cada corrida empuja los cortes con ventas fuera
    // de la ventana. `revisados > 0` abajo es lo que impide que esto pase en vacío, así que la
    // ventana tiene que ser lo bastante ancha para alcanzar un corte de la caja que sí vende.
    const h = await request.get(`${API}/cash-sessions?limit=20`, { headers: auth });
    const cortes = (await h.json()).items as Array<{ id: number }>;

    let revisados = 0;
    for (const c of cortes) {
      const d = await request.get(`${API}/cash-sessions/${c.id}`, { headers: auth });
      const det = await d.json();
      const ventas = (det.sales ?? []) as Array<{ status: string; total: string }>;
      if (ventas.length === 0) continue;
      revisados++;

      const ingreso = ventas
        .filter((v) => v.status !== 'cancelada' && v.status !== 'reembolsada')
        .reduce((s, v) => s + Number(v.total), 0);
      expect(
        Number(det.salesTotal),
        `el corte ${c.id} declara ${det.salesTotal} y sus ventas con ingreso suman ${ingreso}`,
      ).toBeCloseTo(ingreso, 2);
    }
    expect(revisados, 'ningún corte del ambiente traía ventas que revisar').toBeGreaterThan(0);
  });

  // La lista y el conteo salen del mismo `where`. Si divergen, uno de los dos miente y quien lee un
  // arqueo no tiene forma de saber cuál.
  test('U3 · el conteo del corte nunca es menor que lo que muestra', async ({ request }) => {
    const jwt = await tokenDeRequest(request);
    const auth = { Authorization: `Bearer ${jwt}` };
    const h = await request.get(`${API}/cash-sessions?limit=10`, { headers: auth });
    const cortes = (await h.json()).items as Array<{ id: number }>;

    for (const c of cortes.slice(0, 5)) {
      const d = await request.get(`${API}/cash-sessions/${c.id}`, { headers: auth });
      const det = await d.json();
      expect(det.salesShown, `el corte ${c.id} manda más ventas de las que dice tener`)
        .toBeLessThanOrEqual(det.salesCount);
      expect(det.salesShown, `el corte ${c.id} desacuerda consigo mismo`).toBe((det.sales ?? []).length);
    }
  });

  // Lo vendido en un corte tiene que quedar explicado por dos cifras del MISMO corte: lo que entró
  // por método y lo que se entregó sin cobrar. Un peso que no esté en ninguna de las dos es dinero
  // que la pantalla perdió de vista.
  //
  // El tercer sumando existe desde el 8 de septiembre de 2026. Antes este caso comparaba lo vendido
  // contra el esperado a secas, y encontró un corte real que cerró con los diez métodos en
  // diferencia $0.00 mientras cinco pedidos entregados por $554.00 no tenían un solo pago: el
  // arqueo cuadraba porque solo compara pagos contra declarado. Sumar `uncollected` NO afloja el
  // caso — sigue fallando si aparece un peso que no está ni cobrado ni declarado como no cobrado—,
  // lo que hace es exigir que la pantalla NOMBRE el hueco en vez de callarlo.
  test('U2 · las ventas de un corte cuadran con lo que su arqueo espera', async ({ request }) => {
    const jwt = await tokenDeRequest(request);
    const auth = { Authorization: `Bearer ${jwt}` };
    const h = await request.get(`${API}/cash-sessions?limit=10`, { headers: auth });
    const cortes = (await h.json()).items as Array<{ id: number; status: string }>;

    let revisados = 0;
    for (const c of cortes.filter((x) => x.status === 'cerrada').slice(0, 4)) {
      const d = await request.get(`${API}/cash-sessions/${c.id}`, { headers: auth });
      const det = await d.json();
      if ((det.sales ?? []).length === 0) continue;
      revisados++;

      // El esperado incluye el fondo de apertura y las propinas; la venta nunca puede pasarlo.
      const esperado = (det.totals ?? []).reduce(
        (s: number, t: { expected: string }) => s + Number(t.expected), 0);
      const sinCobrar = Number(det.uncollected ?? 0);
      expect(
        Number(det.salesTotal),
        `el corte ${c.id} vendió ${det.salesTotal}, su arqueo espera ${esperado} y declara ` +
        `${sinCobrar} sin cobrar: sobran ${(Number(det.salesTotal) - esperado - sinCobrar).toFixed(2)} ` +
        'que no están ni cobrados ni nombrados',
      ).toBeLessThanOrEqual(esperado + sinCobrar + 0.01);
    }
    // Sin cortes cerrados con ventas no hay nada que cuadrar. Se SALTA en vez de pasar en verde:
    // un test que no midió nada y se reporta como bueno es peor que uno que falta.
    test.skip(revisados === 0, 'el ambiente no tiene cortes cerrados con ventas que cuadrar');
  });
});

test.describe('U — el aviso de turno viejo', () => {
  // El aviso lo decide el SERVIDOR. Si lo decidiera la tableta, dos aparatos con la hora distinta
  // dirían cosas distintas del mismo turno.
  test('U6 · el estado de caja dice desde cuándo está abierta y si ya no es de hoy', async ({ request }) => {
    const jwt = await tokenDeRequest(request);
    const r = await request.get(`${API}/cash-status`, { headers: { Authorization: `Bearer ${jwt}` } });
    expect(r.ok(), `/cash-status respondió ${r.status()}`).toBeTruthy();
    const estado = await r.json();

    expect(typeof estado.open, 'el estado de caja perdió el campo que decide si se puede cobrar').toBe('boolean');
    expect(estado, 'el servidor no está decidiendo si el turno es de otro día').toHaveProperty('deOtroDia');
    if (estado.open) {
      expect(estado.openedAt, 'un aviso sin desde-cuándo no le sirve a quien opera').toBeTruthy();
      const abrio = new Date(estado.openedAt).toLocaleDateString('en-CA', { timeZone: 'America/Mexico_City' });
      const hoy = new Date().toLocaleDateString('en-CA', { timeZone: 'America/Mexico_City' });
      expect(estado.deOtroDia, `abrió el ${abrio}, hoy es ${hoy}: el veredicto no coincide`).toBe(abrio < hoy);
    }
  });
});
