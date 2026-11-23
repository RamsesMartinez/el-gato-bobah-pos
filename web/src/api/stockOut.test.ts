import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { stockOutApi } from './stockOut';

// La búsqueda de «se acabó» marca «ya reportado» por almacén (spec 094): el de venta va SIN
// warehouseId —un ?warehouseId= vacío es 400— y otro almacén con su id.
let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = vi.fn(async () => new Response(JSON.stringify({ items: [] }), {
    status: 200, headers: { 'Content-Type': 'application/json' },
  }));
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => vi.unstubAllGlobals());

const url = () => String((fetchMock.mock.calls[0] as [RequestInfo | URL])[0]);

test('en el de venta busca sin almacén', async () => {
  await stockOutApi.search('leche');
  expect(url()).toMatch(/\/stock\/out-reports\/search\?q=leche$/);
});

test('en otro almacén busca con su id', async () => {
  await stockOutApi.search('leche', 2);
  expect(url()).toMatch(/\/stock\/out-reports\/search\?q=leche&warehouseId=2$/);
});
