import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
import { API, tokenDeRequest } from './ambiente';
import { iniciarSesion } from './sesion';
import { pickDate } from './pickDate';

// EDITAR UN GASTO GUARDADO (spec 053) a 1024×600 contra el `make start` del worktree con la base
// restaurada. El caso que lo motivó (1-nov-2026): una compra de tapas y fajillas guardada sin
// mercancía. Cada prueba crea su propio gasto; lo que mueve en el almacén es de la base local.

const CAPTURAS = process.env.E2E_CAPTURAS ?? 'test-results/editar-gasto';

test.describe.configure({ mode: 'serial', retries: 0 });

let jwt = '';
let category = 0;
let transfer = 0;
let tapas: { id: number; name: string } | null = null;
let fajillas: { id: number; name: string } | null = null;
let pieza = 0;

const h = () => ({ Authorization: `Bearer ${jwt}` });
async function api(request: APIRequestContext, method: 'get' | 'post', path: string, data?: unknown) {
  const r = await request[method](`${API}${path}`, { headers: h(), data });
  if (!r.ok()) throw new Error(`${method.toUpperCase()} ${path}: ${r.status()} ${await r.text()}`);
  const t = await r.text();
  return t ? JSON.parse(t) : null;
}

const hoy = () => new Date().toLocaleDateString('en-CA', { timeZone: 'America/Mexico_City' });
const diasAtras = (n: number) => {
  const d = new Date(`${hoy()}T12:00:00`);
  d.setDate(d.getDate() - n);
  return d.toISOString().slice(0, 10);
};

async function gastoSinMercancia(request: APIRequestContext, description: string, amount: string): Promise<number> {
  const { id } = await api(request, 'post', '/expenses', {
    categoryId: category, amount, status: 'pagada', description, expenseDay: diasAtras(1),
    payments: [{ methodId: transfer, amount }],
  });
  return id;
}

// sumado dice cuánto sumó al almacén el renglón del artículo en el gasto (lo aplicado, en base).
async function sumado(request: APIRequestContext, expenseId: number, ingredientId: number): Promise<number> {
  const d = await api(request, 'get', `/expenses/${expenseId}`);
  const it = d.items.find((x: { ingredientId: number | null }) => x.ingredientId === ingredientId);
  return Number(it?.applied ?? 0);
}

async function abrirGasto(page: Page, description: string) {
  await page.goto('/gastos');
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  // Entrar lleva a «Vender»: se vuelve a Gastos.
  await page.goto('/gastos');
  await page.waitForLoadState('networkidle');
  const fila = page.getByRole('row').filter({ hasText: description });
  await fila.getByRole('button', { name: 'Ver' }).click();
  await expect(page.getByRole('dialog').getByText(/Gasto \d+/).first()).toBeVisible();
}

// teclear escribe en el teclado en pantalla lo que dice `valor` (antes borra lo que hubiera).
async function teclear(page: Page, valor: string) {
  const pad = page.getByTestId('teclado').last();
  for (let i = 0; i < 8; i++) await pad.getByRole('button', { name: 'Borrar' }).click();
  for (const c of valor) await pad.getByRole('button', { name: c === '.' ? 'punto' : c, exact: true }).click();
}

async function agregarRenglon(page: Page, articulo: string, cantidad: string, importe: string) {
  await page.getByRole('button', { name: '+ Agregar renglón' }).click();
  const hoja = page.getByRole('dialog').filter({ hasText: 'Agregar renglón' });
  await hoja.getByRole('button', { name: /¿Qué/ }).click();
  const picker = page.getByRole('dialog').last();
  await picker.getByPlaceholder('Buscar…').fill(articulo.slice(0, 12));
  await picker.getByRole('button', { name: new RegExp(articulo.slice(0, 12), 'i') }).first().click();
  const pz = hoja.getByRole('button', { name: /^Pieza/ });
  if (await pz.isVisible().catch(() => false)) await pz.click();
  await hoja.getByLabel('Cantidad').click();
  await teclear(page, cantidad);
  await hoja.getByRole('button', { name: 'Listo' }).click();
  await hoja.getByLabel('Importe').click();
  await teclear(page, importe);
  await hoja.getByRole('button', { name: 'Listo' }).click();
  await hoja.getByRole('button', { name: 'Agregar', exact: true }).click();
}

test.beforeAll(async ({ request }) => {
  jwt = await tokenDeRequest(request);
  category = (await api(request, 'get', '/expense-categories')).items[0].id;
  transfer = (await api(request, 'get', '/payment-methods')).items.find((m: { affectsCashDrawer: boolean }) => !m.affectsCashDrawer).id;
  pieza = (await api(request, 'get', '/units')).items.find((u: { code: string }) => u.code === 'pieza').id;
  const { items } = await api(request, 'get', '/ingredients?onlyActive=true');
  const enPiezas = items.filter((i: { baseUnitCode: string }) => i.baseUnitCode === 'pieza');
  tapas = enPiezas.find((i: { name: string }) => /tapa/i.test(i.name)) ?? enPiezas[0];
  fajillas = enPiezas.find((i: { name: string }) => /fajilla/i.test(i.name)) ?? enPiezas[1];
  console.log(`[e2e] artículos: ${tapas!.name} y ${fajillas!.name}`);
});

// Criterio 1 y SC-001: el gasto del 1-nov se corrige desde la tableta, sin SQL, en menos de 2 min.
test('agregar la mercancía a un gasto guardado sin ella y recibirla', async ({ page, request }) => {
  const desc = `E2E 053 tapas ${Date.now()}`;
  const id = await gastoSinMercancia(request, desc, '67.50');
  const inicio = Date.now();
  await abrirGasto(page, desc);

  await page.getByRole('button', { name: 'Editar gasto' }).click();
  await agregarRenglon(page, tapas!.name, '50', '50');
  await agregarRenglon(page, fajillas!.name, '10', '17.50');
  await expect(page.getByText('2 cambios sin confirmar')).toBeVisible();
  await page.getByRole('button', { name: 'Revisar cambios' }).click();
  await expect(page.getByText('Listo para confirmar')).toBeVisible();
  await page.waitForTimeout(600);
  await page.screenshot({ path: `${CAPTURAS}/vista-previa-agregar.png` });
  await page.getByRole('button', { name: 'Confirmar' }).click();
  await expect(page.getByText('Listo: el gasto quedó corregido')).toBeVisible();

  await page.getByRole('button', { name: 'Recibir' }).click();
  await expect(page.getByText('2 de 2 renglones')).toBeVisible();
  await page.waitForTimeout(600);
  await page.screenshot({ path: `${CAPTURAS}/hoja-de-entrega.png` });
  await page.getByRole('button', { name: 'Ver efectos' }).click();
  await expect(page.getByRole('cell', { name: tapas!.name, exact: true })).toBeVisible();
  await page.waitForTimeout(600); // la hoja termina de entrar
  await page.screenshot({ path: `${CAPTURAS}/vista-previa-recibir.png` });
  await page.getByRole('button', { name: 'Confirmar' }).click();
  await expect(page.getByText('Listo: quedó registrado lo que llegó').first()).toBeVisible();
  const segundos = Math.round((Date.now() - inicio) / 1000);
  console.log(`[e2e] caso del 1-nov corregido en ${segundos} s`);
  expect(segundos).toBeLessThan(120);
  expect(await sumado(request, id, tapas!.id)).toBeCloseTo(50, 2);
  const d = await api(request, 'get', `/expenses/${id}`);
  expect(d.amount).toBe('67.5');
  expect(d.payments).toHaveLength(1);
});

// Criterio 4: recibir 3 de 5 con fecha de ayer y el resto hoy.
test('recibir en dos entregas, cada una con su fecha', async ({ page, request }) => {
  const desc = `E2E 053 entregas ${Date.now()}`;
  const id = await gastoSinMercancia(request, desc, '50');
  const body = { add: [{ itemType: 'ingrediente', ingredientId: tapas!.id, description: 'Tapas por entregas', quantity: '5', unitId: pieza, amount: '50' }] };
  const pre = await api(request, 'post', `/expenses/${id}/edit?preview=1`, body);
  await api(request, 'post', `/expenses/${id}/edit`, { ...body, fingerprint: pre.preview.fingerprint });

  await abrirGasto(page, desc);
  await page.getByRole('button', { name: 'Recibir' }).click();
  await page.getByLabel(/Cuántos llegaron de/).click();
  await teclear(page, '3');
  await page.getByRole('button', { name: 'Listo' }).click();
  await expect(page.getByText(/Faltarán 2/)).toBeVisible();
  await pickDate(page, 'Día de la entrega', diasAtras(1));
  await page.getByRole('button', { name: 'Ver efectos' }).click();
  await expect(page.getByRole('button', { name: 'Sigue pendiente de entrega' })).toHaveAttribute('aria-pressed', 'true');
  await page.getByRole('button', { name: 'Confirmar' }).click();
  await expect(page.getByText('Listo: quedó registrado lo que llegó').first()).toBeVisible();

  await page.getByRole('button', { name: 'Recibir' }).click();
  await expect(page.getByLabel(/Cuántos llegaron de/)).toHaveValue('2');
  await page.getByRole('button', { name: 'Ver efectos' }).click();
  await page.getByRole('button', { name: 'Confirmar' }).click();
  await expect(page.getByText('Listo: quedó registrado lo que llegó').first()).toBeVisible();
  await page.getByRole('button', { name: 'Entregas (2)' }).click();
  await expect(page.getByTestId('historial-entregas')).toContainText('3 piezas');
  await expect(page.getByTestId('historial-entregas')).toContainText('2 piezas');
});

// Revisión de tableta U2: con dos decisiones, «Confirmar» y lo que falta decidir se ven sin
// desplazar a 1024×600.
test('vista previa con dos decisiones cabe en la tableta', async ({ page, request }) => {
  const desc = `E2E 053 decisiones ${Date.now()}`;
  const id = await gastoSinMercancia(request, desc, '100');
  const body = { add: [
    { itemType: 'ingrediente', ingredientId: tapas!.id, description: 'Tapas de menos', quantity: '5', unitId: pieza, amount: '50' },
    { itemType: 'ingrediente', ingredientId: fajillas!.id, description: 'Fajillas de más', quantity: '1', unitId: pieza, amount: '50' },
  ] };
  const pre = await api(request, 'post', `/expenses/${id}/edit?preview=1`, body);
  await api(request, 'post', `/expenses/${id}/edit`, { ...body, fingerprint: pre.preview.fingerprint });

  await abrirGasto(page, desc);
  await page.getByRole('button', { name: 'Recibir' }).click();
  const filas = page.getByTestId('renglon-entrega');
  await filas.filter({ hasText: tapas!.name }).getByLabel(/Cuántos llegaron de/).click();
  await teclear(page, '3');
  await page.getByRole('button', { name: 'Listo' }).click();
  await filas.filter({ hasText: fajillas!.name }).getByRole('button', { name: /^Más / }).click();
  await filas.filter({ hasText: fajillas!.name }).getByLabel(/Importe de lo de más/).click();
  await teclear(page, '20');
  await page.getByRole('button', { name: 'Listo' }).click();
  await page.getByRole('button', { name: 'Ver efectos' }).click();
  await expect(page.getByTestId('fila-decision')).toHaveCount(2);
  const confirmar = page.getByRole('button', { name: 'Confirmar' });
  await expect(confirmar).toBeInViewport();
  await expect(page.getByRole('button', { name: /Falta decidir 1/ })).toBeInViewport();
  await page.waitForTimeout(600);
  await page.screenshot({ path: `${CAPTURAS}/vista-previa-dos-decisiones.png` });
  await page.getByRole('button', { name: /Falta decidir/ }).click();
  await page.getByRole('button', { name: 'Dejar pendiente' }).click();
  await expect(confirmar).toBeEnabled();
  await confirmar.click();
  await expect(page.getByText('Listo: quedó registrado lo que llegó').first()).toBeVisible();
  await expect(page.getByText('⚠ Falta pagar')).toBeVisible();
});
