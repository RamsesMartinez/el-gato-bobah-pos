import { test, expect, type Locator, type Page } from '@playwright/test';
import { entrar } from './pos';

// EL AVISO DE VERSIÓN NUEVA NO TAPA LA PANTALLA (spec 064).
//
// Era un toast arriba al centro que no se cerraba solo, y a 1024×600 tapó «Agregar lugar» en
// Negocio. Ahora ocupa el lugar del logo en el riel. Lo que jsdom no puede medir: que el área de
// trabajo mida lo mismo con y sin aviso, que ningún botón de la pantalla quede debajo, y que la
// lista del riel no pierda alto (con admin a 600 px apenas le sobran unos píxeles).
//
// En desarrollo no hay service worker: `__egbSimularVersionNueva` prende el aviso como lo haría
// una versión nueva de verdad. Solo existe en desarrollo, así que corre contra `make start`.

test.use({ hasTouch: true, viewport: { width: 1024, height: 600 } });

const PANTALLAS = [
  { riel: 'Vender', ruta: '/pos' },
  { riel: 'Pedidos', ruta: '/pedidos' },
  { riel: 'Negocio', ruta: '/negocio' },
  { riel: 'Caja', ruta: '/caja' },
];

const boton = (page: Page) => page.getByRole('button', { name: 'Actualizar a la versión nueva' });
const caja = async (l: Locator) => {
  const b = await l.boundingBox();
  if (!b) throw new Error('sin caja: no está a la vista');
  return { x: Math.round(b.x), y: Math.round(b.y), w: Math.round(b.width), h: Math.round(b.height) };
};
const seCruzan = (a: { x: number; y: number; w: number; h: number }, b: typeof a) =>
  a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;

async function irA(page: Page, riel: string, ruta: string) {
  // Navegación del riel y no `goto`: recargar borraría el aviso, que vive en la pestaña.
  const link = page.locator('[data-rail-list] a', { hasText: riel });
  await link.scrollIntoViewIfNeeded();
  await link.click();
  await expect(page).toHaveURL(new RegExp(`${ruta}$`));
  await page.waitForLoadState('networkidle');
}

async function enteraEnLaVentana(l: Locator, nombre: string) {
  const b = await caja(l);
  expect(b.y, `${nombre} quedó arriba del borde`).toBeGreaterThanOrEqual(0);
  expect(b.y + b.h, `${nombre} quedó partido bajo el pliegue`).toBeLessThanOrEqual(600);
}

test('el aviso no tapa ni le quita alto a ninguna pantalla', async ({ page }) => {
  await entrar(page);
  const area = page.locator('[data-work-area]');
  const lista = page.locator('[data-rail-list]');

  const sinAviso: Record<string, { area: number; lista: number }> = {};
  for (const { riel, ruta } of PANTALLAS) {
    await irA(page, riel, ruta);
    sinAviso[riel] = { area: (await caja(area)).h, lista: (await caja(lista)).h };
  }

  await expect(boton(page)).toHaveCount(0);
  await page.evaluate(() => window.__egbSimularVersionNueva!());
  await expect(boton(page)).toBeVisible();
  expect((await caja(boton(page))).h, 'el botón no llega a 44 px').toBeGreaterThanOrEqual(44);
  // Ningún toast del mismo hecho.
  await expect(page.getByText('Nueva versión disponible')).toHaveCount(0);

  for (const { riel, ruta } of PANTALLAS) {
    await irA(page, riel, ruta);
    await expect(boton(page), `el aviso desapareció al entrar a ${riel}`).toBeVisible();
    const aviso = await caja(boton(page));
    expect((await caja(area)).h, `${riel}: el aviso le quitó alto a la pantalla`).toBe(sinAviso[riel].area);
    expect((await caja(lista)).h, `${riel}: el aviso le quitó alto a la lista del riel`).toBe(sinAviso[riel].lista);

    const botones = area.getByRole('button');
    for (let i = 0; i < await botones.count(); i++) {
      const b = botones.nth(i);
      if (!(await b.isVisible())) continue;
      const bb = await b.boundingBox();
      if (!bb) continue;
      expect(seCruzan(aviso, { x: bb.x, y: bb.y, w: bb.width, h: bb.height }),
        `${riel}: el aviso tapa «${(await b.textContent())?.trim()}»`).toBe(false);
    }

    await enteraEnLaVentana(page.locator('[data-rail-list] a[aria-current="page"]'), `${riel} (opción abierta)`);
    await enteraEnLaVentana(page.getByRole('button', { name: 'Salir' }), '«Salir»');
    await page.screenshot({ path: `test-results/aviso-de-version/${riel}.png` });
  }

  // El caso del issue: «Agregar lugar» en Negocio, a la vista y sin nada encima.
  await irA(page, 'Negocio', '/negocio');
  const agregar = page.getByRole('button', { name: 'Agregar lugar' });
  await agregar.scrollIntoViewIfNeeded();
  await expect(agregar).toBeVisible();
  const centro = await caja(agregar);
  const encima = await page.evaluate(([x, y]) => document.elementFromPoint(x, y)?.closest('button')?.textContent, [centro.x + centro.w / 2, centro.y + centro.h / 2]);
  expect(encima, 'algo tapa «Agregar lugar»').toContain('Agregar lugar');
});

// Antes de entrar no hay riel: el aviso ocupa el lugar del logo del inicio de sesión. Una versión
// que no deja entrar se arregla pasando a la siguiente, y el formulario sigue cabiendo en 600 px.
test('en el inicio de sesión el aviso ocupa el lugar del logo y el formulario cabe', async ({ page }) => {
  await page.goto('/login');
  await page.waitForLoadState('networkidle');
  await page.evaluate(() => window.__egbSimularVersionNueva!());
  await expect(boton(page)).toBeVisible();
  await expect(page.getByAltText('El Gato Bobah')).toHaveCount(0);
  await enteraEnLaVentana(page.getByRole('button', { name: 'Entrar', exact: true }), '«Entrar»');
  await enteraEnLaVentana(page.getByRole('button', { name: '¿Olvidaste tu contraseña?' }), '«¿Olvidaste tu contraseña?»');
  await page.screenshot({ path: 'test-results/aviso-de-version/login.png' });
});
