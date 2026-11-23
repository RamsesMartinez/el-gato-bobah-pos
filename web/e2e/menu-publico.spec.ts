import { expect, test, type Page } from '@playwright/test';
import { API, tokenDeRequest } from './ambiente';
import { iniciarSesion } from './sesion';

// EL MENÚ PÚBLICO (spec 097), de punta a punta y contra la API real: lo que la dueña ve en Ajustes y
// lo que el cliente abre en su celular. El menú lo sirve la Function local (`bun run dev:menu`), en
// E2E_MENU_URL; sin ella, la parte del cliente se salta.
const MENU = process.env.E2E_MENU_URL ?? '';

async function controlesChicos(page: Page, dentro = 'body') {
  return page.evaluate((sel) => [...document.querySelectorAll(`${sel} a, ${sel} button, ${sel} summary, ${sel} input`)]
    .filter((e) => {
      const b = e.getBoundingClientRect();
      return b.width > 0 && b.height > 0 && b.height < 43.5 && getComputedStyle(e).visibility !== 'hidden';
    })
    .map((e) => (e.getAttribute('aria-label') || e.textContent || e.tagName).trim().slice(0, 40)), dentro);
}

test.describe('Ajustes › Venta en web', () => {
  for (const [nombre, w, h] of [['mostrador', 1280, 854], ['2K', 2560, 1440]] as const) {
    test(`cabe sin desplazar a ${nombre}, aun con el error del nombre visible`, async ({ page }) => {
      await page.setViewportSize({ width: w, height: h });
      await page.goto('/');
      await iniciarSesion(page);
      await page.goto('/negocio?seccion=web');
      await expect(page.getByText('Comparte tu menú en línea')).toBeVisible({ timeout: 30_000 });
      await expect(page.getByRole('img', { name: /código qr/i }).locator('svg')).toBeVisible();
      await page.getByLabel(/nombre en la dirección/i).fill('ab');
      await expect(page.getByText(/usa al menos 3 letras o números/i)).toBeVisible();
      const panel = await page.evaluate(() => {
        const t = [...document.querySelectorAll('div')].find((d) => getComputedStyle(d).overflowY === 'auto' && d.textContent?.includes('Comparte tu menú en línea'))!;
        return { alto: t.scrollHeight, visible: t.clientHeight };
      });
      expect(panel.alto, `la sección mide ${panel.alto} px en ${panel.visible} px de alto útil`).toBeLessThanOrEqual(panel.visible + 1);
      expect(await controlesChicos(page, 'main')).toEqual([]);
    });
  }
});

test.describe('el menú que abre un cliente', () => {
  test.skip(!MENU, 'sin E2E_MENU_URL (bun run dev:menu) no hay menú que abrir');

  let nombre = '';
  test.beforeAll(async ({ request }) => {
    const jwt = await tokenDeRequest(request);
    const r = await fetch(`${API}/admin/public-menu`, { headers: { Authorization: `Bearer ${jwt}` } });
    nombre = ((await r.json()) as { name: string }).name;
  });

  for (const [disp, w, h] of [['celular', 360, 780], ['tableta', 768, 1024], ['computadora', 1280, 854]] as const) {
    test(`se lee y se navega en ${disp}`, async ({ browser }) => {
      const ctx = await browser.newContext({ viewport: { width: w, height: h }, hasTouch: true, isMobile: w < 800 });
      const page = await ctx.newPage();
      const r = await page.goto(`${MENU}/${nombre}`);
      expect(r?.status()).toBe(200);
      expect(await page.locator('script').count()).toBe(0);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      expect(await controlesChicos(page)).toEqual([]);

      // El mosaico: abrir, tocar la última categoría, llegar a su sección.
      await page.locator('.mosb:visible').first().click();
      await expect(page.locator('#categorias')).toBeVisible();
      const ultima = page.locator('#categorias .portada a').last();
      const destino = await ultima.getAttribute('href');
      await ultima.click();
      await expect(page.locator('#categorias')).toBeHidden();
      await expect(page.locator(destino!)).toBeInViewport();

      // Un producto con foto o descripción se abre en su lugar y se cierra con otro toque; uno sin
      // nada más que enseñar no es tocable.
      expect(await page.locator('div.ren').count() + await page.locator('details.ren').count()).toBeGreaterThan(0);
      const prod = page.locator('details.ren').first();
      await prod.scrollIntoViewIfNeeded();
      await prod.locator('summary').click();
      await expect(prod).toHaveAttribute('open', '');
      await prod.locator('summary').click();
      await expect(prod).not.toHaveAttribute('open', '');
      await ctx.close();
    });
  }

  test('las mayúsculas y un nombre que no existe', async ({ request }) => {
    const r = await request.get(`${MENU}/${nombre.toUpperCase()}`, { maxRedirects: 0 });
    expect(r.status()).toBe(302);
    expect(r.headers().location).toBe(`/${nombre}`);
    expect((await request.get(`${MENU}/no-existe-este-menu`, { maxRedirects: 0 })).status()).toBe(404);
  });
});
