import { test, expect, type Page } from '@playwright/test';
import { randomBytes } from 'node:crypto';
import { API, EMPRESA, tokenDeApi } from './ambiente';
import { altaDePrueba, bajaDePrueba, type PersonaDePrueba } from './personas';
import { cuentaNueva, entrar } from './pos';
import { abrirDelMenu, riel, rotulosDelRiel } from './menu';

// EL MENÚ EN DOS NIVELES, POR ROL Y A LOS TAMAÑOS DE VERDAD (spec 085, issue #293).
//
// jsdom no tiene alto: que los módulos y el pie quepan sin desplazar a 1280×854, que a 1280×768 el
// pie siga a la vista y que en 2K se vea todo solo se mide en un navegador. Lo que ve cada rol se
// mide con personas creadas aquí, con los permisos de su rol sugerido; el nombre del rol no decide
// nada en el código.

const MOSTRADOR = { width: 1280, height: 854 };
test.use({ hasTouch: true, viewport: MOSTRADOR });

let admin = '';
let asistencia = false;
const creadas: PersonaDePrueba[] = [];

async function persona(rol: 'cajero' | 'gerente'): Promise<PersonaDePrueba & { password: string }> {
  const p = await altaDePrueba(admin, rol === 'cajero' ? 'Cajera' : 'Gerente', rol);
  creadas.push(p);
  // Nace con cambio de contraseña obligatorio: se cambia por API para que la prueba mida el riel y
  // no la pantalla de cambio.
  const login = await fetch(`${API}/auth/login`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: p.username, slug: EMPRESA, password: p.password }),
  });
  if (!login.ok) throw new Error(`login de ${p.username}: ${login.status}`);
  const jwt = (await login.json()).accessToken as string;
  const nueva = `Menu-${randomBytes(9).toString('hex')}!`;
  const r = await fetch(`${API}/me/password`, {
    method: 'POST', headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ currentPassword: p.password, newPassword: nueva }),
  });
  if (!r.ok) throw new Error(`cambio de contraseña de ${p.username}: ${r.status} ${await r.text()}`);
  return { ...p, password: nueva };
}

async function entrarComo(page: Page, p: { username: string; password: string }) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  await page.getByPlaceholder('usuario@empresa').fill(`${p.username}@${EMPRESA}`);
  await page.getByPlaceholder('Contraseña').fill(p.password);
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(cuentaNueva(page)).toBeVisible({ timeout: 30_000 });
}

// El riel no desplaza: la lista de módulos mide lo que su contenido.
async function sinDesplazar(page: Page, quien: string) {
  const m = await riel(page).evaluate((el) => ({ scroll: el.scrollHeight, alto: el.clientHeight }));
  expect(m.scroll, `${quien}: el riel desplaza (${m.scroll} de ${m.alto} px)`).toBeLessThanOrEqual(m.alto + 1);
}

async function dentroDeLaVentana(page: Page, sel: ReturnType<Page['locator']>, nombre: string) {
  const b = await sel.boundingBox();
  const alto = page.viewportSize()!.height;
  expect(b, `${nombre} no está a la vista`).not.toBeNull();
  expect(b!.y + b!.height, `${nombre} quedó bajo el pliegue`).toBeLessThanOrEqual(alto);
}

test.beforeAll(async () => {
  admin = await tokenDeApi();
  const r = await fetch(`${API}/attendance/roster`, { headers: { Authorization: `Bearer ${admin}` } });
  asistencia = r.ok && ((await r.json()) as { enabled: boolean }).enabled === true;
});

test.afterAll(async () => {
  for (const p of creadas) await bajaDePrueba(admin, p);
});

test('admin a 1280×854: ocho módulos, sin desplazar, con el pie completo', async ({ page }) => {
  await entrar(page);
  expect(await rotulosDelRiel(page)).toEqual(['Vender', 'Pedidos', 'Caja', 'Inventario', 'Menú', 'Personal', 'Reportes', 'Ajustes']);
  await sinDesplazar(page, 'admin');
  await dentroDeLaVentana(page, page.getByRole('button', { name: 'Salir' }), '«Salir»');
  expect((await page.locator('[data-bell-slot]').boundingBox())!.height).toBeGreaterThanOrEqual(44);
  for (const b of await riel(page).locator(':scope > div > a, :scope > div > button').all()) {
    expect((await b.boundingBox())!.height, `${await b.textContent()} mide menos de 72 px`).toBeGreaterThanOrEqual(72);
  }
  await page.screenshot({ path: 'test-results/menu-por-rol/admin-1280.png' });
});

// Debajo de 854 px el riel desplaza como red: Salir y el módulo abierto siguen a la vista.
test('admin a 1280×768: Salir y el módulo abierto a la vista', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 768 });
  await entrar(page);
  await page.goto('/cuenta');
  await page.waitForLoadState('networkidle');
  await dentroDeLaVentana(page, page.getByRole('button', { name: 'Salir' }), '«Salir»');
  const activo = riel(page).locator('[data-active="true"]');
  await expect(activo).toHaveText(/Ajustes/);
  const lista = (await riel(page).boundingBox())!;
  const caja = (await activo.boundingBox())!;
  expect(caja.y >= lista.y - 1 && caja.y + caja.height <= lista.y + lista.height + 1,
    `Ajustes no se ve completo en el riel (${Math.round(caja.y)}–${Math.round(caja.y + caja.height)} de ${Math.round(lista.y)}–${Math.round(lista.y + lista.height)})`).toBe(true);
  await page.screenshot({ path: 'test-results/menu-por-rol/admin-768.png' });
});

test('el panel del gerente lleva a Gastos, Asistencia e Interfaz', async ({ page }) => {
  const g = await persona('gerente');
  await entrarComo(page, g);
  const esperados = ['Vender', 'Pedidos', 'Caja', 'Inventario', 'Menú', 'Personal', 'Reportes', 'Ajustes'];
  if (!asistencia) esperados.splice(esperados.indexOf('Personal'), 1);
  expect(await rotulosDelRiel(page)).toEqual(esperados);
  await sinDesplazar(page, 'gerente');

  await abrirDelMenu(page, 'Gastos');
  await expect(page).toHaveURL(/\/gastos$/);
  if (asistencia) {
    await abrirDelMenu(page, 'Asistencia');
    await expect(page).toHaveURL(/\/asistencia$/);
  }
  await abrirDelMenu(page, 'Interfaz');
  await expect(page).toHaveURL(/\/apariencia$/);

  // Escape cierra el panel sin navegar.
  await riel(page).getByRole('button', { name: 'Caja', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Caja' })).toBeVisible();
  await page.screenshot({ path: 'test-results/menu-por-rol/gerente-panel-caja.png' });
  await page.keyboard.press('Escape');
  await expect(page.getByRole('navigation', { name: 'Caja' })).toHaveCount(0);
  await expect(page).toHaveURL(/\/apariencia$/);
});

test('el cajero ve lo suyo y abre Caja de un toque', async ({ page }) => {
  const c = await persona('cajero');
  await entrarComo(page, c);
  const esperados = ['Vender', 'Pedidos', 'Caja', 'Inventario', 'Personal', 'Ajustes'];
  if (!asistencia) esperados.splice(esperados.indexOf('Personal'), 1);
  expect(await rotulosDelRiel(page)).toEqual(esperados);
  await sinDesplazar(page, 'cajero');
  await expect(riel(page).getByRole('link', { name: 'Caja', exact: true })).toHaveAttribute('href', '/caja');
  await page.screenshot({ path: 'test-results/menu-por-rol/cajero-1280.png' });

  // Cada ruta que el cajero abre hoy sigue abriendo por URL directa.
  for (const ruta of ['/pos', '/pedidos', '/caja', '/merma', '/apariencia', '/cuenta']) {
    await page.goto(ruta);
    await page.waitForLoadState('networkidle');
    await expect(page, `${ruta} no abrió`).toHaveURL(new RegExp(`${ruta}$`));
  }
});

test('en 2K el menú se ve expandido, sin desplazar', async ({ page }) => {
  await page.setViewportSize({ width: 2560, height: 1440 });
  await entrar(page);
  for (const l of ['Ventas', 'Gastos', 'Almacén', 'Plataformas', 'Empleados', 'Negocio', 'Impresión', 'Integraciones', 'Interfaz', 'Mi cuenta']) {
    await expect(riel(page).getByRole('link', { name: l, exact: true }), l).toBeInViewport();
  }
  await expect(riel(page).getByRole('button')).toHaveCount(0);
  await sinDesplazar(page, 'admin en 2K');
  await page.screenshot({ path: 'test-results/menu-por-rol/admin-2k.png' });
});

// Ninguna ruta cambió (FR-009): cada una abre la misma pantalla por URL directa, redirecciones
// incluidas, y su módulo queda marcado.
test('cada ruta de hoy abre por URL directa', async ({ page }) => {
  await entrar(page);
  const rutas: Array<[string, RegExp, string]> = [
    ['/pos', /\/pos$/, 'Vender'], ['/pedidos', /\/pedidos$/, 'Pedidos'], ['/caja', /\/caja$/, 'Caja'],
    ['/caja/administrar', /\/caja\/administrar$/, 'Caja'], ['/ventas', /\/ventas$/, 'Caja'], ['/gastos', /\/gastos$/, 'Caja'],
    ['/almacen', /\/almacen$/, 'Inventario'], ['/almacen?tab=conteo', /\/almacen\?tab=conteo$/, 'Inventario'],
    ['/merma', /\/merma$/, 'Inventario'], ['/catalogo', /\/catalogo\/productos$/, 'Menú'],
    ['/productos', /\/catalogo\/productos$/, 'Menú'], ['/opciones', /\/catalogo\/opciones$/, 'Menú'],
    ['/plataformas', /\/plataformas$/, 'Menú'], ['/empleados', /\/empleados$/, 'Personal'],
    ['/reportes', /\/reportes$/, 'Reportes'], ['/negocio', /\/negocio$/, 'Ajustes'],
    ['/negocio#lugares', /\/negocio#lugares$/, 'Ajustes'], ['/impresion', /\/impresion$/, 'Ajustes'],
    ['/integraciones', /\/integraciones$/, 'Ajustes'], ['/apariencia', /\/apariencia$/, 'Ajustes'],
    ['/cuenta', /\/cuenta$/, 'Ajustes'],
  ];
  if (asistencia) rutas.push(['/asistencia', /\/asistencia$/, 'Personal']);
  for (const [ruta, final, modulo] of rutas) {
    await page.goto(ruta);
    await page.waitForLoadState('networkidle');
    await expect(page, `${ruta} no abrió`).toHaveURL(final);
    await expect(riel(page).locator('[data-active="true"]'), `${ruta} no marca ${modulo}`).toHaveText(new RegExp(`^${modulo}`));
  }
});
