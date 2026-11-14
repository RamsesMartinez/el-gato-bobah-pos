import { test, expect, type Page } from '@playwright/test';
import { tokenDeApi } from './ambiente';
import { LOCAL, pedir, entrarACaja, type Caja } from './cortes';

// CAJA EN CADA PANTALLA (#290, fase 4; FR-035 a FR-038, Maquetas M1 y M2).
//
// Caja tenía un tope de 920 px: en un monitor de 1920 se usaba menos de la mitad y en 2560 un tercio.
// Aquí se mide en cinco tamaños, de la tableta vertical al monitor 2K, lo que jsdom no ve porque no
// tiene layout: que nada pida desplazar a los lados, que los controles se acierten con el dedo, que
// en pantallas grandes el contenido ocupe la pantalla y que la lista del Histórico no corte nombres ni
// fechas en 1366. Las capturas van a `test-results/`, nunca al repositorio.
const CAPTURAS = process.env.E2E_CAPTURAS ?? 'test-results/caja-anchos';
const RIEL = 76;
// El mínimo de la constitución y del tema compartido. El plan pedía 48 en Caja; se quedó en 44 porque
// pestañas, chips y campos salen de los componentes comunes, y subirlos solo aquí haría que Caja se
// viera de otro producto (constitución, «un solo estilo»). Los botones propios de Caja miden 48 o más.
const CONTROL_MINIMO = 44;

const TAMANOS = [
  { nombre: 'tableta-vertical', width: 768, height: 1024, ocupa: null },
  { nombre: 'tableta', width: 1024, height: 600, ocupa: null },
  { nombre: 'laptop', width: 1366, height: 768, ocupa: null },
  { nombre: 'fhd', width: 1920, height: 1080, ocupa: 0.85 },
  { nombre: '2k', width: 2560, height: 1440, ocupa: 0.85 },
] as const;

// medir: desborde horizontal de la página, controles chicos del área de contenido y ancho que ocupa
// el panel de la pestaña frente al ancho útil (la ventana menos el riel).
async function medir(page: Page) {
  return page.evaluate(({ riel, minimo }) => {
    const visible = (e: Element) => {
      const r = e.getBoundingClientRect();
      const s = getComputedStyle(e);
      return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && r.left >= riel && r.top < innerHeight && r.bottom > 0;
    };
    const controles = [...document.querySelectorAll('button, [role="tab"], input:not([type="hidden"]), textarea')]
      .filter(visible)
      .filter((e) => !e.closest('[role="dialog"]'))
      .map((e) => ({ nombre: (e.getAttribute('aria-label') ?? e.textContent ?? '').trim().slice(0, 40), alto: Math.round(e.getBoundingClientRect().height) }))
      .filter((c) => c.alto < minimo);
    const panel = document.querySelector('[role="tabpanel"]:not([hidden])');
    return {
      desborde: document.documentElement.scrollWidth - innerWidth,
      chicos: controles,
      ocupa: panel ? panel.getBoundingClientRect().width / (innerWidth - riel) : 0,
    };
  }, { riel: RIEL, minimo: CONTROL_MINIMO });
}

let principal: Caja | undefined;

test.beforeAll(async () => {
  test.skip(!LOCAL, 'mide con la caja principal abierta: solo contra la base local');
  const jwt = await tokenDeApi();
  const cajas = ((await pedir(jwt, 'GET', '/cash-registers')) as { items: Caja[] }).items;
  principal = cajas.find((c) => c.isPrimary && c.openSessionId !== null);
});

for (const t of TAMANOS) {
  test(`A · ${t.width}×${t.height}: sin desplazar a los lados, controles de ${CONTROL_MINIMO} px${t.ocupa ? ' y la pantalla aprovechada' : ''}`, async ({ page }) => {
    test.skip(!principal, 'la caja principal no está abierta');
    await page.setViewportSize({ width: t.width, height: t.height });
    await entrarACaja(page);

    const pantallas: Array<{ nombre: string; ir: () => Promise<void> }> = [
      { nombre: 'turno', ir: async () => {
        await page.goto(`/caja/${principal!.id}`);
        await expect(page.getByText('Cómo va el turno')).toBeVisible({ timeout: 30_000 });
      } },
      { nombre: 'cierre', ir: async () => {
        await page.goto(`/caja/${principal!.id}/cerrar`);
        await expect(page.getByRole('navigation', { name: 'Pasos del cierre' })).toBeVisible({ timeout: 30_000 });
      } },
      { nombre: 'historico', ir: async () => {
        await page.goto('/caja/historico');
        await expect(page.getByRole('row').nth(1)).toBeVisible({ timeout: 30_000 });
      } },
    ];

    const fallas: string[] = [];
    for (const p of pantallas) {
      await p.ir();
      await page.waitForTimeout(300);
      const m = await medir(page);
      await page.screenshot({ path: `${CAPTURAS}/${t.nombre}-${p.nombre}.png` });
      console.log(`[medición] ${t.width}×${t.height} ${p.nombre}: desborde ${m.desborde}px · ocupa ${Math.round(m.ocupa * 100)}% · ${m.chicos.length} controles bajo ${CONTROL_MINIMO}px`);
      if (m.desborde > 1) fallas.push(`${p.nombre}: la página se desplaza ${m.desborde}px a los lados`);
      for (const c of m.chicos) fallas.push(`${p.nombre}: «${c.nombre}» mide ${c.alto}px`);
      if (t.ocupa && m.ocupa < t.ocupa) fallas.push(`${p.nombre}: ocupa ${Math.round(m.ocupa * 100)}% del ancho útil`);
    }
    expect(fallas, fallas.join('\n')).toEqual([]);
  });
}

// En 1366 la lista va al lado del detalle (340–400 px): nombre, quién abrió, quién cerró y «Ver» se
// leen completos; un renglón cortado obliga a abrir cada corte para saber cuál es.
test('H · en 1366 la lista del Histórico no corta lo que dice', async ({ page }) => {
  await page.setViewportSize({ width: 1366, height: 768 });
  await entrarACaja(page);
  await page.goto('/caja/historico');
  await expect(page.getByRole('row').nth(1)).toBeVisible({ timeout: 30_000 });
  const cortadas = await page.evaluate(() =>
    [...document.querySelectorAll('tbody tr:nth-child(-n+5) td *')]
      // Solo bloques: un elemento en línea reporta clientWidth 0 y parecería cortado siempre.
      .filter((e) => e.children.length === 0 && getComputedStyle(e).display !== 'inline' && e.scrollWidth > e.clientWidth + 1)
      .map((e) => (e.textContent ?? '').trim()));
  expect(cortadas, `textos cortados: ${cortadas.join(' · ')}`).toEqual([]);
  await expect(page.getByRole('row').nth(1).getByRole('button', { name: 'Ver' })).toBeInViewport();
});
