import { describe, expect, test } from 'vitest';

import { shellFromRoot, workboxOptions } from './workboxOptions';

// ENTRAR DIRECTO A UNA RUTA DABA ERR_FAILED (2026-11-07). El service worker servía las navegaciones
// con el «index.html» de su caché; si esa entrada faltaba (cambio de versión, caché desalojado) la
// pedía a la red, y Cloudflare Pages responde /index.html con 308 a «/». Una respuesta redirigida no
// sirve para una navegación: la tableta quedaba en blanco al recargar /pedidos tras una subida. El
// shell se precarga y se sirve como «/», que nunca redirige.
describe('el shell del service worker es «/», nunca «index.html»', () => {
  test('las navegaciones caen en «/»', () => {
    expect(workboxOptions.navigateFallback).toBe('/');
  });

  test('la entrada index.html del precache se publica como «/», con su misma revisión', async () => {
    const { manifest } = await shellFromRoot([
      { url: 'index.html', revision: 'abc', size: 1 },
      { url: 'assets/app.js', revision: null, size: 2 },
    ]);
    expect(manifest).toEqual([
      { url: '/', revision: 'abc', size: 1 },
      { url: 'assets/app.js', revision: null, size: 2 },
    ]);
  });

  test('ninguna entrada se llama index.html', async () => {
    const { manifest } = await shellFromRoot([{ url: 'index.html', revision: 'x', size: 1 }]);
    expect(manifest.some((e) => e.url.endsWith('index.html'))).toBe(false);
  });
});
