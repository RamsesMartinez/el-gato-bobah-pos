import { test, expect, type Page } from '@playwright/test';
import { API, tokenDeApi } from './ambiente';
import { iniciarSesion } from './sesion';

// MERMA ESPERADA, SUBIDA 2 (spec 096) contra la API real del worktree, a 1024×600 (pedido del dueño)
// y a 1280×854: la opción de descontar la merma, el conteo en la vista sencilla con su avance, la
// avanzada con lo esperado, y Existencias en tablero. Deja todo como estaba: la opción en su valor y
// el conteo que abrió, descartado. Si ya había un conteo abierto, no lo toca (es de alguien).
// Las capturas van a test-results/, nunca al repositorio.
const CAPTURAS = process.env.E2E_CAPTURAS ?? 'test-results/merma-esperada-s2';

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const jwt = await tokenDeApi();
  const r = await fetch(`${API}${path}`, { ...init, headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json', ...(init?.headers ?? {}) } });
  if (!r.ok) throw new Error(`${init?.method ?? 'GET'} ${path}: ${r.status}`);
  const text = await r.text();
  return (text ? JSON.parse(text) : undefined) as T;
}
const setDeducts = (on: boolean) => api('/settings/expected-waste-deducts', { method: 'PUT', body: JSON.stringify({ deducts: on }) });

let deductsAntes = false;
let conteoAjeno = false;

test.beforeAll(async () => {
  deductsAntes = !!(await api<{ expectedWasteDeducts?: boolean }>('/business-settings')).expectedWasteDeducts;
  conteoAjeno = !!(await api<{ id: number } | null>('/stock/counts/current'));
});

test.afterAll(async () => {
  await setDeducts(deductsAntes);
});

async function entrar(page: Page, path: string) {
  await page.goto(path);
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  await page.goto(path);
}

for (const [w, h] of [[1024, 600], [1280, 854]] as const) {
  test.describe(`a ${w}×${h}`, () => {
    test.use({ viewport: { width: w, height: h }, hasTouch: true });

    test('la opción de descontar la merma se enciende con un toque y sobrevive a recargar', async ({ page }) => {
      await setDeducts(false);
      await entrar(page, '/negocio?seccion=inventario');
      const sw = page.getByRole('checkbox', { name: 'La merma esperada también descuenta del almacén' });
      await expect(sw).toBeAttached({ timeout: 15_000 });
      await page.getByText('La merma esperada también descuenta del almacén').scrollIntoViewIfNeeded();
      await page.locator('label').filter({ has: sw }).tap();
      await expect.poll(async () => (await api<{ expectedWasteDeducts?: boolean }>('/business-settings')).expectedWasteDeducts).toBe(true);
      await page.reload();
      await expect(page.locator('label').filter({ has: sw })).toHaveAttribute('data-state', 'checked', { timeout: 15_000 });
      await page.screenshot({ path: `${CAPTURAS}/ajustes-descontar-${w}.png` });
    });

    test('Existencias en tablero: tres columnas, sin desplazar la página a lo ancho', async ({ page }) => {
      await entrar(page, '/almacen?tab=existencias');
      await page.getByRole('button', { name: 'Sencilla' }).first().tap();
      for (const col of ['Se acabó', 'Queda poco', 'Hay']) await expect(page.getByRole('region', { name: col })).toBeVisible({ timeout: 15_000 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(w);
      await page.screenshot({ path: `${CAPTURAS}/existencias-sencilla-${w}.png` });
      await page.getByRole('button', { name: 'Avanzada' }).first().tap();
      await expect(page.getByTestId('valor-total')).toBeVisible();
    });

    test('el conteo en la sencilla: un lugar, tres artículos, el avance sube y la avanzada enseña lo esperado', async ({ page }) => {
      test.skip(conteoAjeno, 'ya había un conteo abierto: no se toca el de alguien más');
      await entrar(page, '/almacen?tab=conteo');
      await page.getByRole('button', { name: /^Empezar (otro )?conteo$/ }).tap();
      const avance = page.getByTestId('avance');
      await expect(avance).toBeVisible({ timeout: 20_000 });
      try {
        await page.getByRole('button', { name: 'Sencilla' }).first().tap();
        // El nombre del artículo en la tarjeta (el otro h2 es el título de la página).
        const tarjeta = page.getByRole('heading', { level: 2 }).filter({ hasNotText: /^Almacén$/ });
        for (let i = 0; i < 3; i++) {
          const nombre = await tarjeta.textContent();
          await page.getByRole('button', { name: '2', exact: true }).tap();
          await page.getByRole('button', { name: 'Guardar y seguir ›' }).tap();
          // Una cifra rara pide confirmarla; aquí se confirma (es una prueba, no un conteo de verdad).
          // Se espera lo que llegue primero: el aviso o el siguiente artículo.
          const si = page.getByRole('button', { name: 'Sí, guardar' });
          await expect.poll(async () => {
            if (await si.isVisible().catch(() => false)) await si.tap();
            return (await tarjeta.textContent({ timeout: 500 }).catch(() => nombre)) !== nombre;
          }, { timeout: 15_000 }).toBe(true);
        }
        await expect(avance).toHaveText(/^3 de \d+ contados$/);
        // Lo que no cabe a 1024 no se esconde: el botón de guardar y el teclado siguen dentro de la pantalla.
        const guardar = await page.getByRole('button', { name: 'Guardar y seguir ›' }).boundingBox();
        expect(guardar && guardar.y + guardar.height).toBeLessThanOrEqual(h);
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(w);
        await page.screenshot({ path: `${CAPTURAS}/conteo-sencilla-${w}.png` });
        await page.getByRole('button', { name: 'Avanzada' }).first().tap();
        await expect(page.getByText(/^Esperado /).first()).toBeVisible({ timeout: 15_000 });
        await expect(page.getByText(/^Diferencia /).first()).toBeVisible();
        await page.screenshot({ path: `${CAPTURAS}/conteo-avanzada-${w}.png` });
      } finally {
        const abierto = await api<{ id: number } | null>('/stock/counts/current');
        if (abierto) await api(`/stock/counts/${abierto.id}/discard`, { method: 'POST' });
      }
    });
  });
}
