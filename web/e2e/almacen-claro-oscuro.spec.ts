import { test, expect, type Page } from '@playwright/test';
import { entrar } from './pos';
import { iniciarSesion } from './sesion';

// ALMACÉN EN CLARO Y EN OSCURO (spec 044, US7). En tema oscuro la lista de Existencias no se leía:
// los renglones con existencia negativa llevaban un fondo fijo claro (`red.50`) con el texto del tema
// (claro en oscuro). jsdom no resuelve el tema, así que se mide aquí: contraste de cada texto de la
// tabla contra su fondo real, con la regla de WCAG (4.5:1). Solo lee.

test.use({ hasTouch: true });

const CAPTURAS = process.env.E2E_CAPTURAS ?? 'test-results/almacen-claro-oscuro';

async function conTema(page: Page, tema: 'light' | 'dark') {
  // next-themes lee su elección de localStorage («theme») antes de pintar.
  await page.addInitScript((t) => { try { localStorage.setItem('theme', t); } catch { /* sin almacenamiento */ } }, tema);
  await entrar(page);
  await page.goto('/almacen?tab=existencias');
  await expect(page.locator('table tbody tr').first()).toBeVisible({ timeout: 15_000 });
}

// sinContraste devuelve los textos de la tabla cuyo contraste contra su fondo efectivo es menor que
// 4.5:1, con el texto, la cifra y los dos colores: el fallo dice cuál y por qué.
async function sinContraste(page: Page) {
  return page.evaluate(() => {
    const rgb = (c: string) => (c.match(/[\d.]+/g) ?? []).map(Number);
    const lum = ([r, g, b]: number[]) => {
      const f = (v: number) => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; };
      return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
    };
    const fondo = (e: Element | null): string => {
      for (; e; e = e.parentElement) {
        const c = getComputedStyle(e).backgroundColor;
        const p = rgb(c);
        if (p.length >= 3 && (p.length < 4 || p[3] > 0.5)) return c;
      }
      return 'rgb(255, 255, 255)';
    };
    const malos: { texto: string; ratio: number; color: string; fondo: string }[] = [];
    for (const el of document.querySelectorAll('table th, table td, table td *')) {
      const propio = [...el.childNodes].some((n) => n.nodeType === Node.TEXT_NODE && n.textContent!.trim());
      // Las otras pestañas siguen en el DOM, ocultas: solo cuenta lo que se ve.
      if (!propio || el.getClientRects().length === 0) continue;
      const color = getComputedStyle(el).color;
      const bg = fondo(el);
      const [a, b] = [lum(rgb(color)), lum(rgb(bg))];
      const ratio = (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
      if (ratio < 4.5) malos.push({ texto: (el as HTMLElement).innerText.trim().slice(0, 40), ratio: Math.round(ratio * 100) / 100, color, fondo: bg });
    }
    return malos;
  });
}

for (const tema of ['light', 'dark'] as const) {
  test(`Existencias se lee en tema ${tema === 'dark' ? 'oscuro' : 'claro'}`, async ({ page }) => {
    await conTema(page, tema);
    await expect(page.locator('html')).toHaveClass(new RegExp(tema));
    await page.screenshot({ path: `${CAPTURAS}/existencias-${tema === 'dark' ? 'oscuro' : 'claro'}.png` });
    expect(await sinContraste(page), 'textos de Existencias con contraste menor que 4.5:1').toEqual([]);
  });
}

// Límites del conteo (US6) en los dos temas: la sección se ve y sus controles miden 44 px. Solo lee.
for (const tema of ['light', 'dark'] as const) {
  test(`Límites del conteo en tema ${tema === 'dark' ? 'oscuro' : 'claro'}`, async ({ page }) => {
    await page.addInitScript((t) => { try { localStorage.setItem('theme', t); } catch { /* sin almacenamiento */ } }, tema);
    await entrar(page);
    await page.goto('/negocio');
    const seccion = page.getByTestId('limites-conteo');
    await seccion.scrollIntoViewIfNeeded({ timeout: 15_000 });
    await expect(seccion.getByLabel('Máximo por conteo de Peso (kg)')).toHaveValue(/\d/);
    await page.screenshot({ path: `${CAPTURAS}/limites-${tema === 'dark' ? 'oscuro' : 'claro'}.png` });
    const bajos = await seccion.locator('input, button').evaluateAll((els) => els
      .map((e) => Math.round(e.getBoundingClientRect().height)).filter((h) => h > 0 && h < 44));
    expect(bajos, 'controles de menos de 44 px').toEqual([]);
  });
}

// RECORRIDO DEL ALMACÉN (spec 044, T029). Todas las pestañas en el orden de la pantalla, más la
// ficha de un artículo, el teclado del conteo y Configuración del negocio › Almacén, en los dos temas:
// contraste del texto (4.5:1) y del borde de los campos (3:1) contra su fondo real, desborde de lado y
// alto de lo tocable. El conteo se abre solo si no había uno y se descarta al terminar.

type Hallazgo = { tipo: string; texto: string; cifra: number; color?: string; fondo?: string };

async function medir(page: Page): Promise<Hallazgo[]> {
  return page.evaluate(() => {
    const rgb = (c: string) => (c.match(/[\d.]+/g) ?? []).map(Number);
    const lum = ([r, g, b]: number[]) => {
      const f = (v: number) => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; };
      return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
    };
    const opaco = (c: string) => { const p = rgb(c); return p.length >= 3 && (p.length < 4 || p[3] > 0.5); };
    const fondo = (e: Element | null): string => {
      for (; e; e = e.parentElement) { const c = getComputedStyle(e).backgroundColor; if (opaco(c)) return c; }
      return getComputedStyle(document.body).backgroundColor;
    };
    const ratio = (a: string, b: string) => { const [x, y] = [lum(rgb(a)), lum(rgb(b))]; return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05); };
    const visible = (e: Element) => {
      const r = e.getBoundingClientRect();
      return r.width > 0 && r.height > 0 && r.bottom > 0 && r.top < innerHeight && getComputedStyle(e).visibility !== 'hidden';
    };
    const deshabilitado = (e: Element) => !!e.closest('[disabled],[aria-disabled="true"],[data-disabled]');
    const out: { tipo: string; texto: string; cifra: number; color?: string; fondo?: string }[] = [];
    for (const el of document.querySelectorAll('body *')) {
      if (!visible(el) || deshabilitado(el) || el.closest('svg')) continue;
      const s = getComputedStyle(el);
      const propio = [...el.childNodes].some((n) => n.nodeType === Node.TEXT_NODE && n.textContent!.trim());
      const texto = ((el as HTMLElement).innerText || (el as HTMLInputElement).value || (el as HTMLInputElement).placeholder || el.getAttribute('aria-label') || '').trim().slice(0, 40);
      if (propio && opaco(s.color)) {
        const r = ratio(s.color, fondo(el));
        if (r < 4.5) out.push({ tipo: 'texto', texto, cifra: Math.round(r * 100) / 100, color: s.color, fondo: fondo(el) });
      }
      if (el.matches('input:not([type=checkbox]):not([type=radio]),textarea') && parseFloat(s.borderBottomWidth) > 0 && opaco(s.borderBottomColor)) {
        const r = ratio(s.borderBottomColor, fondo(el.parentElement));
        if (r < 3) out.push({ tipo: 'borde', texto, cifra: Math.round(r * 100) / 100, color: s.borderBottomColor, fondo: fondo(el.parentElement) });
      }
      if (el.matches('button,input,[role=button],[role=tab],[role=switch],textarea')) {
        const h = Math.round(el.getBoundingClientRect().height);
        if (h < 44 && !/^v[0-9a-f]{7}$/.test(texto)) out.push({ tipo: 'alto', texto, cifra: h });
      }
    }
    const lado = document.documentElement.scrollWidth - document.documentElement.clientWidth;
    if (lado > 0) out.push({ tipo: 'de lado', texto: 'página', cifra: lado });
    return out;
  });
}

const DIR_CAPTURAS = process.env.E2E_CAPTURAS_044 ?? '../specs/044-ajustes-de-articulos-y-conteo/capturas';

for (const tema of ['light', 'dark'] as const) {
  const nombre = tema === 'dark' ? 'oscuro' : 'claro';
  test(`recorrido del almacén en tema ${nombre}`, async ({ page }) => {
    test.setTimeout(180_000);
    await page.setViewportSize({ width: 1024, height: 600 });
    await page.addInitScript((t) => { try { localStorage.setItem('theme', t); } catch { /* sin almacenamiento */ } }, tema);
    await entrar(page);
    const hallazgos: Record<string, Hallazgo[]> = {};
    const paso = async (clave: string) => {
      await page.waitForLoadState('networkidle');
      await page.screenshot({ path: `${DIR_CAPTURAS}/${clave}-${nombre}.png` });
      const h = await medir(page);
      if (h.length) hallazgos[clave] = h;
    };
    const pestaña = async (tab: string, listo: () => Promise<unknown>) => {
      await page.goto(`/almacen?tab=${tab}`);
      await listo();
    };
    await pestaña('existencias', () => expect(page.locator('table tbody tr').first()).toBeVisible({ timeout: 15_000 }));
    await paso('01-existencias');
    await pestaña('movimientos', () => page.waitForTimeout(1500));
    await paso('02-movimientos');
    await pestaña('insumos', () => expect(page.locator('tr[role="button"]:visible').first()).toBeVisible({ timeout: 15_000 }));
    await paso('03-articulos');
    await page.locator('tr[role="button"]:visible').first().click();
    await expect(page.getByRole('button', { name: /Volver a Artículos/ })).toBeVisible();
    // La captura a media entrada salía translúcida y parecía que la lista se veía a través de la hoja.
    await page.locator('[role="dialog"]').last().evaluate((d) => Promise.all(d.getAnimations().map((a) => a.finished)));
    expect(await page.locator('[role="dialog"]').last().evaluate((d) => {
      const s = getComputedStyle(d);
      return { opacidad: s.opacity, fondo: s.backgroundColor };
    }), 'la ficha tapa la lista').toEqual({ opacidad: '1', fondo: expect.stringMatching(/^rgb\(/) });
    await paso('04-ficha');
    await pestaña('por-recibir', () => page.waitForTimeout(1500));
    await paso('05-por-recibir');
    await pestaña('conteo', () => page.waitForTimeout(1500));
    let abrimosConteo = false;
    const empezar = page.getByRole('button', { name: /^Empezar (otro )?conteo$/ });
    if (await empezar.isVisible().catch(() => false)) { await empezar.click(); abrimosConteo = true; }
    await expect(page.getByTestId('avance')).toBeVisible({ timeout: 15_000 });
    await paso('06-conteo');
    await page.getByRole('button', { name: 'Lista', exact: true }).click();
    await page.getByTestId('renglon-conteo').first().getByRole('button', { name: /^(Contar|Cambiar)$/ }).click();
    const hoja = page.locator('[role="dialog"]').last();
    await hoja.locator('input').first().click();
    await paso('07-conteo-teclado');
    await hoja.getByRole('button', { name: 'Cancelar' }).click();
    if (abrimosConteo) {
      await page.getByRole('button', { name: 'Descartar conteo' }).click();
      await page.getByRole('button', { name: 'Sí, descartar' }).click();
    }
    await pestaña('merma', () => page.waitForTimeout(1500));
    await paso('08-se-echo-a-perder');
    await pestaña('salud', () => page.waitForTimeout(2500));
    await paso('09-salud');
    await page.goto('/negocio');
    await expect(page.getByTestId('limites-conteo')).toBeAttached({ timeout: 15_000 });
    await paso('10-negocio');
    await page.getByTestId('limites-conteo').scrollIntoViewIfNeeded();
    await paso('11-limites-del-conteo');
    console.log(`[e2e] hallazgos ${nombre}:\n${JSON.stringify(hallazgos, null, 1)}`);
    expect(hallazgos, `defectos del almacén en tema ${nombre}`).toEqual({});
  });
}

// Spec 045 (US5) · SIN ELECCIÓN GUARDADA, MANDA EL SISTEMA. Con «light» por omisión, pedir modo
// oscuro al sistema no cambiaba nada. Se prueba con el navegador limpio y también que siga en vivo
// cuando el sistema cambia con la app abierta (B9).
test.describe('tema del sistema', () => {
  test.use({ colorScheme: 'dark' });
  test('sin elección guardada, abre en oscuro y sigue al sistema en vivo', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await iniciarSesion(page);
    await page.getByRole('link', { name: 'Almacén' }).waitFor();
    expect(await page.evaluate(() => localStorage.getItem('theme'))).toBeNull();
    await expect(page.locator('html')).toHaveClass(/dark/);
    await page.emulateMedia({ colorScheme: 'light' });
    await expect(page.locator('html')).not.toHaveClass(/dark/);
  });
});
