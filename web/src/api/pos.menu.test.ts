import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { posApi, olvidarMenu } from './pos';

// La tableta revisa el menú cada minuto. Si no mandara el ETag, cada revisión bajaría el catálogo
// completo; si un 304 no devolviera el menú que ya tenía, la pantalla de venta se quedaría vacía.
beforeEach(() => olvidarMenu());
afterEach(() => vi.unstubAllGlobals());

const menuA = { version: 1, categories: [], products: [{ id: 1 }] };

test('la segunda petición manda If-None-Match y un 304 devuelve el menú anterior', async () => {
  const fetchMock = vi.fn()
    .mockResolvedValueOnce(new Response(JSON.stringify(menuA), { status: 200, headers: { ETag: '"v3-2-7-d"' } }))
    .mockResolvedValueOnce(new Response(null, { status: 304, headers: { ETag: '"v3-2-7-d"' } }));
  vi.stubGlobal('fetch', fetchMock);

  expect(await posApi.menu()).toEqual(menuA);
  const segundo = await posApi.menu();

  const headers = fetchMock.mock.calls[1][1].headers as Record<string, string>;
  expect(headers['If-None-Match']).toBe('"v3-2-7-d"');
  expect(segundo).toEqual(menuA);
});

test('un ETag nuevo reemplaza el menú', async () => {
  const menuB = { ...menuA, products: [] };
  vi.stubGlobal('fetch', vi.fn()
    .mockResolvedValueOnce(new Response(JSON.stringify(menuA), { status: 200, headers: { ETag: '"a"' } }))
    .mockResolvedValueOnce(new Response(JSON.stringify(menuB), { status: 200, headers: { ETag: '"b"' } })));
  await posApi.menu();
  expect(await posApi.menu()).toEqual(menuB);
});

test('sin menú guardado no se manda If-None-Match', async () => {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(menuA), { status: 200 }));
  vi.stubGlobal('fetch', fetchMock);
  await posApi.menu();
  const headers = fetchMock.mock.calls[0][1].headers as Record<string, string>;
  expect(headers['If-None-Match']).toBeUndefined();
});
