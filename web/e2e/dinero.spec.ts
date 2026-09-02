import { test, expect, type Page, type APIRequestContext } from '@playwright/test';

// LA MATRIZ DE DINERO, EXTREMO A EXTREMO. Ver docs/matriz-de-cobro.md, sección E.
//
// Lo que estas pruebas atrapan y las de vitest no: el desacuerdo entre lo que la pantalla calcula y
// lo que el servidor cobra. Todos los defectos caros de este sistema fueron de esa forma —una hoja
// ofreciendo $115 de un pedido de $95, una barra diciendo $2,141 mientras su lista decía $1,928— y
// ninguno se ve con el backend mockeado.

const API = process.env.E2E_API_URL ?? 'https://api-dev.elgatobobah.com/api/v1';
const USUARIO = process.env.E2E_USER ?? 'admin';
const EMPRESA = process.env.E2E_SLUG ?? 'gatobobah';
const PASSWORD = process.env.E2E_PASSWORD ?? 'Dev-ffb903b3dfb31073!';

async function token(request: APIRequestContext): Promise<string> {
  const r = await request.post(`${API}/auth/login`, {
    data: { username: USUARIO, slug: EMPRESA, password: PASSWORD },
  });
  expect(r.ok(), 'el login del ambiente de pruebas falló').toBeTruthy();
  return (await r.json()).accessToken;
}

// La sesión se siembra por API y no tecleando en la pantalla de login: lo que estas pruebas miden es
// el cobro, y hacerlas pasar por el login las vuelve dependientes de una pantalla que ya tiene sus
// propias pruebas.
async function entrar(page: Page, jwt: string) {
  await page.addInitScript((t) => {
    window.localStorage.setItem('pos.session', JSON.stringify({ state: { token: t }, version: 0 }));
  }, jwt);
  await page.goto('/');
}

test.describe('E — el dinero, de la pantalla al servidor', () => {
  test('E0 · el ambiente responde y la sesión sirve', async ({ page, request }) => {
    const jwt = await token(request);
    const abiertos = await request.get(`${API}/orders/open`, {
      headers: { Authorization: `Bearer ${jwt}` },
    });
    expect(abiertos.ok()).toBeTruthy();

    // La suma de la lista y la cifra del encabezado salen del MISMO predicado. Si divergen, el
    // operador ve dos cifras del mismo dinero y no tiene cómo saber cuál miente.
    const { items, outstanding } = await abiertos.json();
    const suma = items.reduce((s: number, o: { outstanding: string }) => s + Number(o.outstanding), 0);
    expect(Math.abs(suma - Number(outstanding)), 'el total del servidor no es la suma de su lista')
      .toBeLessThan(0.011);

    await entrar(page, jwt);
    await expect(page).toHaveURL(/\/(pos)?$/);
  });

  test('E3 · un cobro repetido no se registra dos veces', async ({ request }) => {
    const jwt = await token(request);
    const auth = { Authorization: `Bearer ${jwt}` };

    const menu = await (await request.get(`${API}/pos/menu`, { headers: auth })).json();
    const prod = menu.products.filter((p: { price: string }) => Number(p.price) > 0)
      .sort((a: { price: string }, b: { price: string }) => Number(a.price) - Number(b.price))[0];
    const metodos = await (await request.get(`${API}/payment-methods`, { headers: auth })).json();
    const efectivo = metodos.items.find(
      (m: { kind: string; deliveryPlatformId: number | null }) =>
        m.kind === 'efectivo' && m.deliveryPlatformId === null);

    const creado = await request.post(`${API}/orders`, {
      headers: auth,
      data: {
        clientUuid: crypto.randomUUID(),
        serviceType: 'mostrador',
        lines: [{ productId: prod.id, qty: 1 }],
      },
    });
    expect(creado.ok()).toBeTruthy();
    const pedido = await creado.json();

    // El MISMO cobro dos veces, con la misma llave: es el doble tap sobre una tableta que no pintó
    // la respuesta. El segundo tiene que ser inocuo.
    const llave = crypto.randomUUID();
    const cobro = {
      methodId: efectivo.id, amount: Number(pedido.total), clientUuid: llave,
    };
    const uno = await request.post(`${API}/orders/${pedido.id}/pay`, { headers: auth, data: cobro });
    const dos = await request.post(`${API}/orders/${pedido.id}/pay`, { headers: auth, data: cobro });
    expect(uno.ok()).toBeTruthy();
    expect(dos.ok(), 'el reintento del mismo cobro debe ser inocuo, no un error').toBeTruthy();
    expect((await dos.json()).yaEstaba, 'el segundo cobro no se reconoció como reintento').toBe(true);

    const detalle = await (await request.get(`${API}/orders/${pedido.id}`, { headers: auth })).json();
    expect(Number(detalle.outstanding), 'quedó saldo tras cobrar el total').toBe(0);

    // Y el mismo cobro con OTRO método se rechaza: si el primero entró y su respuesta se perdió, el
    // operador puede cambiar de método y reintentar. Darlo por hecho deja el cajón descuadrado en
    // los dos métodos a la vez.
    const tarjeta = metodos.items.find(
      (m: { kind: string; deliveryPlatformId: number | null }) =>
        m.kind !== 'efectivo' && m.deliveryPlatformId === null);
    const otro = await request.post(`${API}/orders/${pedido.id}/pay`, {
      headers: auth, data: { ...cobro, methodId: tarjeta.id },
    });
    expect(otro.status(), 'la misma llave con otro método pasó').toBeGreaterThanOrEqual(400);
  });

  test('E5 · un pedido de plataforma no cobra el envío del negocio', async ({ request }) => {
    const jwt = await token(request);
    const auth = { Authorization: `Bearer ${jwt}` };

    const menu = await (await request.get(`${API}/pos/menu`, { headers: auth })).json();
    const prod = menu.products.filter((p: { price: string }) => Number(p.price) > 0)[0];
    const plataforma = menu.platforms?.[0];
    test.skip(!plataforma, 'el ambiente de pruebas no tiene plataformas configuradas');

    // El caso que dejaba el pedido creado y sin cobrar: la pantalla marcaba domicilio, después se
    // asignaba la plataforma, y el total de la pantalla llevaba $20 que el servidor no cobra.
    const creado = await request.post(`${API}/orders`, {
      headers: auth,
      data: {
        clientUuid: crypto.randomUUID(),
        serviceType: 'domicilio',
        deliveryPlatformId: plataforma.id,
        deliveryFee: 20,
        lines: [{ productId: prod.id, qty: 1 }],
      },
    });
    expect(creado.ok()).toBeTruthy();
    const pedido = await creado.json();
    expect(Number(pedido.deliveryFee), 'el servidor cobró un envío de plataforma').toBe(0);
    // Y el detalle dice con qué lista se armó, para que la hoja de cobro ofrezca los métodos buenos.
    expect(pedido.deliveryPlatformId).toBe(plataforma.id);
  });

  test('E2 · repartir entre tres deja el pedido saldado, sin centavos colgando', async ({ request }) => {
    const jwt = await token(request);
    const auth = { Authorization: `Bearer ${jwt}` };

    const menu = await (await request.get(`${API}/pos/menu`, { headers: auth })).json();
    const prod = menu.products.filter((p: { price: string }) => Number(p.price) > 0)[0];
    const metodos = await (await request.get(`${API}/payment-methods`, { headers: auth })).json();
    const mostrador = metodos.items.filter(
      (m: { deliveryPlatformId: number | null }) => m.deliveryPlatformId === null);

    const creado = await request.post(`${API}/orders`, {
      headers: auth,
      data: {
        clientUuid: crypto.randomUUID(), serviceType: 'mostrador',
        lines: [{ productId: prod.id, qty: 1 }],
      },
    });
    const pedido = await creado.json();
    const total = Number(pedido.total);

    // El reparto con el residuo en la ÚLTIMA parte: `total/3` redondeado suma menos que el total y
    // deja un centavo que nadie puede cobrar.
    const centavos = Math.round(total * 100);
    const parte = Math.floor(centavos / 3) / 100;
    const partes = [parte, parte, Math.round((centavos - Math.round(parte * 100) * 2)) / 100];

    let falta = total;
    for (let i = 0; i < partes.length; i++) {
      const r = await request.post(`${API}/orders/${pedido.id}/pay`, {
        headers: auth,
        data: {
          methodId: mostrador[i % mostrador.length].id,
          amount: partes[i],
          clientUuid: crypto.randomUUID(),
        },
      });
      expect(r.ok(), `el cobro ${i + 1} de 3 falló: ${r.status()} ${await r.text()}`).toBeTruthy();
      falta = Math.round((falta - partes[i]) * 100) / 100;
      const res = await r.json();
      // La cifra que devuelve el cobro es la que la pantalla pinta: si no cuadra, el operador ve un
      // faltante que no existe.
      expect(Math.abs(Number(res.outstanding) - Math.max(0, falta))).toBeLessThan(0.011);
    }

    const detalle = await (await request.get(`${API}/orders/${pedido.id}`, { headers: auth })).json();
    expect(detalle.paid, 'tres partes que suman el total no saldaron el pedido').toBe(true);
    expect(Number(detalle.outstanding), 'quedó un centavo que nadie puede cobrar').toBe(0);

    // Y la barra del POS tampoco le ve deuda: cerrar con un predicado y mostrar deuda con otro es
    // de donde salió el centavo fantasma.
    const abiertos = await (await request.get(`${API}/orders/open`, { headers: auth })).json();
    const enLaBarra = abiertos.items.find((o: { id: number }) => o.id === pedido.id);
    if (enLaBarra) {
      expect(Number(enLaBarra.outstanding), 'la barra le ve deuda a un pedido saldado').toBe(0);
    }
  });
});
