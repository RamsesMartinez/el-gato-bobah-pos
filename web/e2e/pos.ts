import { expect, type Page } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { API } from './ambiente';
import { iniciarSesion } from './sesion';

// Lo que comparten los specs que capturan en Vender (spec 030): entrar, poner un producto, abrir el
// ticket y cobrar. Antes cada spec traía su copia y se rompían todos a la vez cuando cambiaba un
// rótulo.

// entrar deja la pantalla de Vender lista. La señal es el «+» de la fila de cuentas: existe siempre
// que el POS cargó, haya o no cuentas abiertas.
export async function entrar(page: Page) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  await expect(cuentaNueva(page)).toBeVisible({ timeout: 30_000 });
}

export const cuentaNueva = (page: Page) => page.getByRole('button', { name: 'Cuenta nueva', exact: true });

// buscar llega al producto por el BUSCADOR, no por el mosaico: con el catálogo real el producto no
// está a la vista al entrar. Con el panel del ticket abierto el buscador es una lupa que despliega
// el campo.
export async function buscar(page: Page, texto: string) {
  let campo = page.getByPlaceholder('Buscar producto…');
  if (!(await campo.isVisible().catch(() => false))) {
    const lupa = page.getByRole('button', { name: 'Buscar producto' });
    if (await lupa.isVisible().catch(() => false)) await lupa.click();
    campo = page.getByPlaceholder('Buscar producto…');
  }
  await campo.fill(texto);
}

// ponerUnProducto toca el producto y, si abre la hoja de modificadores, la confirma. Espera a que el
// servidor lo confirme: un renglón «Guardando…» todavía no se puede mandar.
export async function ponerUnProducto(page: Page, nombre = 'Dedos de Queso Pza') {
  await buscar(page, nombre);
  await page.getByText(nombre).first().click({ timeout: 30_000 });
  const confirmar = page.getByRole('button', { name: /^(Agregar|Confirmar)/ });
  if (await confirmar.isVisible().catch(() => false)) await confirmar.click();
  await expect(page.getByText('Guardando…')).toHaveCount(0, { timeout: 15_000 });
  await cerrarBusqueda(page);
}

// cerrarBusqueda deja de buscar, como el operador después de tocar el producto. Con el panel del
// ticket abierto el campo de búsqueda ocupa el lugar de la fila de cuentas: mientras tenga texto, la
// fila no está en pantalla y cualquier afirmación sobre ella falla por algo que no es la fila.
//
// Con el panel cerrado el campo no tapa la fila, pero conserva el texto, y al abrir el panel la tapa:
// por eso también se vacía.
export async function cerrarBusqueda(page: Page) {
  const cerrar = page.getByRole('button', { name: 'Cerrar búsqueda' });
  if (await cerrar.isVisible().catch(() => false)) { await cerrar.click(); return; }
  const limpiar = page.getByRole('button', { name: 'Limpiar', exact: true });
  if (await limpiar.isVisible().catch(() => false)) await limpiar.click();
}

// nombreDeLaCuentaActiva: el nombre de la cuenta que ESTA tableta tiene abierta, leído de su ficha.
//
// No se toma «la primera cuenta en captura» del servidor: el ambiente es compartido, otras suites
// capturan a la vez, y la primera de la lista es la de otro.
export async function nombreDeLaCuentaActiva(page: Page): Promise<string> {
  await cerrarBusqueda(page);
  const activa = fichas(page).and(page.locator('[aria-pressed="true"]'));
  await expect(activa).toBeVisible({ timeout: 15_000 });
  const etiqueta = (await activa.getAttribute('aria-label')) ?? '';
  return etiqueta.split(' · ')[0];
}

// abrirTicket: a 1024×600 el ticket puede ser un panel, una píldora o una barra abajo según el ancho
// que quede. Se abre por el que esté.
//
// Se ESPERA a que aparezca uno de los dos: justo después de un `goto` la cuenta todavía se está
// cargando, y mirar una sola vez dejaba el panel cerrado y al test buscando el número del pedido
// en una pantalla que no lo enseña.
export async function abrirTicket(page: Page) {
  const ocultar = page.getByRole('button', { name: 'Ocultar pedido' });
  const resumen = page.getByRole('button', { name: /art ·|falta \$|^Ver pedido$/ });
  // En bucle y con toques cortos: al llegar por `?pedido=` la pantalla abre el panel sola, y una
  // píldora que se vio hace un instante puede ya no estar cuando llega el toque.
  await expect(async () => {
    if (await ocultar.isVisible().catch(() => false)) return;
    await resumen.first().click({ timeout: 2_000 }).catch(() => {});
    await expect(ocultar).toBeVisible({ timeout: 2_000 });
  }).toPass({ timeout: 30_000 });
}

// El botón de cobrar: «Enviar y cobrar $X» con algo nuevo, «Cobrar $X» sin nada nuevo. En la píldora
// y en la barra de abajo va sin la cifra. Se toma el que se vea.
export const botonCobrar = (page: Page) =>
  page.getByRole('button', { name: /^(Enviar y cobrar|Cobrar)( \$[\d,.]+)?$/ }).filter({ visible: true }).first();

export const botonEnviar = (page: Page) => page.getByRole('button', { name: /^Enviar (\d+ )?a cocina$/ }).first();

// La fila de cuentas y sus fichas completas.
export const fila = (page: Page) => page.getByLabel('Cuentas', { exact: true });
export const fichas = (page: Page) => fila(page).locator('[data-ficha="true"]');

// productoPorNombre busca el id en el menú del servidor: los casos que crean cuentas por la API lo
// necesitan y no deben depender de en qué posición del mosaico quedó.
export async function productoPorNombre(jwt: string, nombre: string): Promise<number> {
  const r = await fetch(`${API}/pos/menu`, { headers: { Authorization: `Bearer ${jwt}` } });
  const menu = await r.json() as { products?: Array<{ id: number; name: string; groups?: unknown[] }> };
  const p = (menu.products ?? []).find((x) => x.name === nombre);
  if (!p) throw new Error(`el producto «${nombre}» no está en el menú del ambiente`);
  return p.id;
}

// crearCuentaPorApi abre una cuenta en captura como lo haría otra tableta. La limpieza de la suite
// la descarta al final (limpiar-lo-que-cree.ts).
export async function crearCuentaPorApi(jwt: string, productId: number, over: { orderId?: number | null } = {}) {
  const id = randomUUID();
  const r = await fetch(`${API}/pos/drafts`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({
      id, orderId: over.orderId ?? null,
      lines: [{ opId: randomUUID(), productId, qty: '1', modifiers: [], notes: '' }],
    }),
  });
  if (!r.ok) throw new Error(`crear cuenta: ${r.status}`);
  return (await r.json()) as { id: string; folioName: string | null; orderId: number | null };
}

// mandarPorApi manda una cuenta a cocina como lo haría otra tableta.
export async function mandarPorApi(jwt: string, draftId: string) {
  const r = await fetch(`${API}/pos/drafts/${draftId}/send`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' },
    body: '{}',
  });
  if (!r.ok) throw new Error(`mandar a cocina: ${r.status}`);
  return (await r.json()) as { order: { id: number; number: number; folioName: string; total: string } };
}

// alto mide un control en píxeles reales: jsdom no resuelve las clases de Chakra.
export async function alto(page: Page, nombre: string | RegExp) {
  const caja = await page.getByRole('button', { name: nombre }).first().boundingBox();
  return caja?.height ?? 0;
}

const cab = (jwt: string) => ({ Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' });

export interface PedidoDelServidor {
  id: number; number: number; folioName: string; status: string; total: string; outstanding: string;
  paid: boolean; deliveryPlatformId: number | null;
  lines?: Array<{ id: number; productName: string; quantity: string; delivered: string; cancelled: boolean }>;
}

export async function pedido(jwt: string, id: number): Promise<PedidoDelServidor> {
  const r = await fetch(`${API}/orders/${id}`, { headers: cab(jwt) });
  if (!r.ok) throw new Error(`pedido ${id}: ${r.status}`);
  return r.json();
}

export async function entregarPorApi(jwt: string, id: number) {
  const r = await fetch(`${API}/orders/${id}/deliver`, { method: 'POST', headers: cab(jwt), body: '{}' });
  if (!r.ok) throw new Error(`entregar ${id}: ${r.status}`);
}

// pagarPorApi cobra en efectivo lo que se le diga (todo lo que falta si no se dice), como lo haría
// otra tableta.
export async function pagarPorApi(jwt: string, id: number, monto?: number) {
  const metodos = await (await fetch(`${API}/payment-methods`, { headers: cab(jwt) })).json();
  const efectivo = ((metodos.items ?? metodos) as Array<{ id: number; kind: string; deliveryPlatformId: number | null }>)
    .find((m) => m.kind === 'efectivo' && m.deliveryPlatformId === null);
  if (!efectivo) throw new Error('el ambiente no tiene efectivo de mostrador');
  const falta = Number((await pedido(jwt, id)).outstanding);
  const r = await fetch(`${API}/orders/${id}/pay`, {
    method: 'POST', headers: cab(jwt),
    body: JSON.stringify({ methodId: efectivo.id, amount: monto ?? falta, tip: 0, clientUuid: randomUUID() }),
  });
  if (!r.ok) throw new Error(`cobrar ${id}: ${r.status}`);
}

// pedidoPorApi: una cuenta creada y mandada a cocina, como desde otra tableta.
export async function pedidoPorApi(jwt: string, nombre = 'Dedos de Queso Pza', qty = '1') {
  const productId = await productoPorNombre(jwt, nombre);
  const id = randomUUID();
  const r = await fetch(`${API}/pos/drafts`, {
    method: 'POST', headers: cab(jwt),
    body: JSON.stringify({ id, orderId: null, lines: [{ opId: randomUUID(), productId, qty, modifiers: [], notes: '' }] }),
  });
  if (!r.ok) throw new Error(`crear cuenta: ${r.status}`);
  return (await mandarPorApi(jwt, id)).order;
}
