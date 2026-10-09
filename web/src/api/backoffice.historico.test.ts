import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { backofficeApi } from './backoffice';

// El histórico de cortes pide su página y su rango: sin ellos, el servidor contesta la primera
// página y un corte viejo no aparece nunca.
let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = vi.fn(async () => new Response(JSON.stringify({ items: [], total: 0 }), {
    status: 200, headers: { 'Content-Type': 'application/json' },
  }));
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => vi.unstubAllGlobals());

const url = () => String((fetchMock.mock.calls[0] as [RequestInfo | URL])[0]);

test('manda página, tamaño y el rango de días', async () => {
  await backofficeApi.cashHistory({ page: 2, pageSize: 20, from: '2026-08-01', to: '2026-08-31' });
  expect(url()).toMatch(/\/cash-sessions\?page=2&pageSize=20&from=2026-08-01&to=2026-08-31$/);
});

test('sin rango no manda fechas vacías', async () => {
  await backofficeApi.cashHistory({ page: 0, pageSize: 20 });
  expect(url()).toMatch(/\/cash-sessions\?page=0&pageSize=20$/);
});
