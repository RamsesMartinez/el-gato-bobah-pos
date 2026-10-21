import { test, expect, type Page } from '@playwright/test';
import { entrar } from './pos';

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
