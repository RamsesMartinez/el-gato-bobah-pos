# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: dinero.spec.ts >> E — el dinero, de la pantalla al servidor >> E5 · un pedido de plataforma no cobra el envío del negocio
- Location: e2e/dinero.spec.ts:102:3

# Error details

```
Error: expect(received).toBe(expected) // Object.is equality

Expected: 1
Received: undefined
```

# Test source

```ts
  27  |   await page.addInitScript((t) => {
  28  |     window.localStorage.setItem('pos.session', JSON.stringify({ state: { token: t }, version: 0 }));
  29  |   }, jwt);
  30  |   await page.goto('/');
  31  | }
  32  | 
  33  | test.describe('E — el dinero, de la pantalla al servidor', () => {
  34  |   test('E0 · el ambiente responde y la sesión sirve', async ({ page, request }) => {
  35  |     const jwt = await token(request);
  36  |     const abiertos = await request.get(`${API}/orders/open`, {
  37  |       headers: { Authorization: `Bearer ${jwt}` },
  38  |     });
  39  |     expect(abiertos.ok()).toBeTruthy();
  40  | 
  41  |     // La suma de la lista y la cifra del encabezado salen del MISMO predicado. Si divergen, el
  42  |     // operador ve dos cifras del mismo dinero y no tiene cómo saber cuál miente.
  43  |     const { items, outstanding } = await abiertos.json();
  44  |     const suma = items.reduce((s: number, o: { outstanding: string }) => s + Number(o.outstanding), 0);
  45  |     expect(Math.abs(suma - Number(outstanding)), 'el total del servidor no es la suma de su lista')
  46  |       .toBeLessThan(0.011);
  47  | 
  48  |     await entrar(page, jwt);
  49  |     await expect(page).toHaveURL(/\/(pos)?$/);
  50  |   });
  51  | 
  52  |   test('E3 · un cobro repetido no se registra dos veces', async ({ request }) => {
  53  |     const jwt = await token(request);
  54  |     const auth = { Authorization: `Bearer ${jwt}` };
  55  | 
  56  |     const menu = await (await request.get(`${API}/pos/menu`, { headers: auth })).json();
  57  |     const prod = menu.products.filter((p: { price: string }) => Number(p.price) > 0)
  58  |       .sort((a: { price: string }, b: { price: string }) => Number(a.price) - Number(b.price))[0];
  59  |     const metodos = await (await request.get(`${API}/payment-methods`, { headers: auth })).json();
  60  |     const efectivo = metodos.items.find(
  61  |       (m: { kind: string; deliveryPlatformId: number | null }) =>
  62  |         m.kind === 'efectivo' && m.deliveryPlatformId === null);
  63  | 
  64  |     const creado = await request.post(`${API}/orders`, {
  65  |       headers: auth,
  66  |       data: {
  67  |         clientUuid: crypto.randomUUID(),
  68  |         serviceType: 'mostrador',
  69  |         lines: [{ productId: prod.id, qty: 1 }],
  70  |       },
  71  |     });
  72  |     expect(creado.ok()).toBeTruthy();
  73  |     const pedido = await creado.json();
  74  | 
  75  |     // El MISMO cobro dos veces, con la misma llave: es el doble tap sobre una tableta que no pintó
  76  |     // la respuesta. El segundo tiene que ser inocuo.
  77  |     const llave = crypto.randomUUID();
  78  |     const cobro = {
  79  |       methodId: efectivo.id, amount: Number(pedido.total), clientUuid: llave,
  80  |     };
  81  |     const uno = await request.post(`${API}/orders/${pedido.id}/pay`, { headers: auth, data: cobro });
  82  |     const dos = await request.post(`${API}/orders/${pedido.id}/pay`, { headers: auth, data: cobro });
  83  |     expect(uno.ok()).toBeTruthy();
  84  |     expect(dos.ok(), 'el reintento del mismo cobro debe ser inocuo, no un error').toBeTruthy();
  85  |     expect((await dos.json()).yaEstaba, 'el segundo cobro no se reconoció como reintento').toBe(true);
  86  | 
  87  |     const detalle = await (await request.get(`${API}/orders/${pedido.id}`, { headers: auth })).json();
  88  |     expect(Number(detalle.outstanding), 'quedó saldo tras cobrar el total').toBe(0);
  89  | 
  90  |     // Y el mismo cobro con OTRO método se rechaza: si el primero entró y su respuesta se perdió, el
  91  |     // operador puede cambiar de método y reintentar. Darlo por hecho deja el cajón descuadrado en
  92  |     // los dos métodos a la vez.
  93  |     const tarjeta = metodos.items.find(
  94  |       (m: { kind: string; deliveryPlatformId: number | null }) =>
  95  |         m.kind !== 'efectivo' && m.deliveryPlatformId === null);
  96  |     const otro = await request.post(`${API}/orders/${pedido.id}/pay`, {
  97  |       headers: auth, data: { ...cobro, methodId: tarjeta.id },
  98  |     });
  99  |     expect(otro.status(), 'la misma llave con otro método pasó').toBeGreaterThanOrEqual(400);
  100 |   });
  101 | 
  102 |   test('E5 · un pedido de plataforma no cobra el envío del negocio', async ({ request }) => {
  103 |     const jwt = await token(request);
  104 |     const auth = { Authorization: `Bearer ${jwt}` };
  105 | 
  106 |     const menu = await (await request.get(`${API}/pos/menu`, { headers: auth })).json();
  107 |     const prod = menu.products.filter((p: { price: string }) => Number(p.price) > 0)[0];
  108 |     const plataforma = menu.platforms?.[0];
  109 |     test.skip(!plataforma, 'el ambiente de pruebas no tiene plataformas configuradas');
  110 | 
  111 |     // El caso que dejaba el pedido creado y sin cobrar: la pantalla marcaba domicilio, después se
  112 |     // asignaba la plataforma, y el total de la pantalla llevaba $20 que el servidor no cobra.
  113 |     const creado = await request.post(`${API}/orders`, {
  114 |       headers: auth,
  115 |       data: {
  116 |         clientUuid: crypto.randomUUID(),
  117 |         serviceType: 'domicilio',
  118 |         deliveryPlatformId: plataforma.id,
  119 |         deliveryFee: 20,
  120 |         lines: [{ productId: prod.id, qty: 1 }],
  121 |       },
  122 |     });
  123 |     expect(creado.ok()).toBeTruthy();
  124 |     const pedido = await creado.json();
  125 |     expect(Number(pedido.deliveryFee), 'el servidor cobró un envío de plataforma').toBe(0);
  126 |     // Y el detalle dice con qué lista se armó, para que la hoja de cobro ofrezca los métodos buenos.
> 127 |     expect(pedido.deliveryPlatformId).toBe(plataforma.id);
      |                                       ^ Error: expect(received).toBe(expected) // Object.is equality
  128 |   });
  129 | 
  130 |   test('E2 · repartir entre tres deja el pedido saldado, sin centavos colgando', async ({ request }) => {
  131 |     const jwt = await token(request);
  132 |     const auth = { Authorization: `Bearer ${jwt}` };
  133 | 
  134 |     const menu = await (await request.get(`${API}/pos/menu`, { headers: auth })).json();
  135 |     const prod = menu.products.filter((p: { price: string }) => Number(p.price) > 0)[0];
  136 |     const metodos = await (await request.get(`${API}/payment-methods`, { headers: auth })).json();
  137 |     const mostrador = metodos.items.filter(
  138 |       (m: { deliveryPlatformId: number | null }) => m.deliveryPlatformId === null);
  139 | 
  140 |     const creado = await request.post(`${API}/orders`, {
  141 |       headers: auth,
  142 |       data: {
  143 |         clientUuid: crypto.randomUUID(), serviceType: 'mostrador',
  144 |         lines: [{ productId: prod.id, qty: 1 }],
  145 |       },
  146 |     });
  147 |     const pedido = await creado.json();
  148 |     const total = Number(pedido.total);
  149 | 
  150 |     // El reparto con el residuo en la ÚLTIMA parte: `total/3` redondeado suma menos que el total y
  151 |     // deja un centavo que nadie puede cobrar.
  152 |     const centavos = Math.round(total * 100);
  153 |     const parte = Math.floor(centavos / 3) / 100;
  154 |     const partes = [parte, parte, Math.round((centavos - Math.round(parte * 100) * 2)) / 100];
  155 | 
  156 |     let falta = total;
  157 |     for (let i = 0; i < partes.length; i++) {
  158 |       const r = await request.post(`${API}/orders/${pedido.id}/pay`, {
  159 |         headers: auth,
  160 |         data: {
  161 |           methodId: mostrador[i % mostrador.length].id,
  162 |           amount: partes[i],
  163 |           clientUuid: crypto.randomUUID(),
  164 |         },
  165 |       });
  166 |       expect(r.ok(), `el cobro ${i + 1} de 3 falló`).toBeTruthy();
  167 |       falta = Math.round((falta - partes[i]) * 100) / 100;
  168 |       const res = await r.json();
  169 |       // La cifra que devuelve el cobro es la que la pantalla pinta: si no cuadra, el operador ve un
  170 |       // faltante que no existe.
  171 |       expect(Math.abs(Number(res.outstanding) - Math.max(0, falta))).toBeLessThan(0.011);
  172 |     }
  173 | 
  174 |     const detalle = await (await request.get(`${API}/orders/${pedido.id}`, { headers: auth })).json();
  175 |     expect(detalle.paid, 'tres partes que suman el total no saldaron el pedido').toBe(true);
  176 |     expect(Number(detalle.outstanding), 'quedó un centavo que nadie puede cobrar').toBe(0);
  177 | 
  178 |     // Y la barra del POS tampoco le ve deuda: cerrar con un predicado y mostrar deuda con otro es
  179 |     // de donde salió el centavo fantasma.
  180 |     const abiertos = await (await request.get(`${API}/orders/open`, { headers: auth })).json();
  181 |     const enLaBarra = abiertos.items.find((o: { id: number }) => o.id === pedido.id);
  182 |     if (enLaBarra) {
  183 |       expect(Number(enLaBarra.outstanding), 'la barra le ve deuda a un pedido saldado').toBe(0);
  184 |     }
  185 |   });
  186 | });
  187 | 
```