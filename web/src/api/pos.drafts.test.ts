import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { posApi } from './pos';

// La frontera con el servidor de la cuenta en captura (contracts/api.md). Cada función arma la URL,
// el método y el cuerpo que el contrato congela: un campo con otro nombre no truena en ningún lado,
// el servidor lo ignora y la cuenta se queda sin el dato.

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn(async () => new Response(JSON.stringify({}), {
    status: 200, headers: { 'Content-Type': 'application/json' },
  }));
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => vi.unstubAllGlobals());

function llamada(i = 0): { url: string; method: string; body: unknown } {
  const [url, init] = fetchMock.mock.calls[i] as [RequestInfo | URL, RequestInit | undefined];
  const raw = init?.body;
  return {
    url: String(url),
    method: init?.method ?? 'GET',
    body: typeof raw === 'string' ? JSON.parse(raw) : raw,
  };
}

describe('cuentas en captura', () => {
  test('crear manda id, pedido, nombre propuesto, cabecera y el primer renglón', async () => {
    const body = {
      id: 'd-1', orderId: null, folioName: 'Levkoy',
      header: { serviceType: 'mostrador' as const, platformId: null },
      lines: [{ opId: 'op-1', productId: 41, qty: '1', modifiers: [], notes: '' }],
    };
    await posApi.createDraft(body);
    const c = llamada();
    expect(c.method).toBe('POST');
    expect(c.url).toMatch(/\/pos\/drafts$/);
    expect(c.body).toEqual(body);
  });

  test('leer una cuenta', async () => {
    await posApi.getDraft('d-1');
    expect(llamada().method).toBe('GET');
    expect(llamada().url).toMatch(/\/pos\/drafts\/d-1$/);
  });

  test('agregar un producto y el «+» de un renglón van al mismo endpoint', async () => {
    await posApi.addDraftLine('d-1', { opId: 'op-2', productId: 41, qty: '1', modifiers: [], notes: '' });
    await posApi.addDraftLine('d-1', { opId: 'op-3', intoLineId: 'l-1', qty: '1' });
    expect(llamada(0).url).toMatch(/\/pos\/drafts\/d-1\/lines$/);
    expect(llamada(0).method).toBe('POST');
    expect(llamada(1).body).toEqual({ opId: 'op-3', intoLineId: 'l-1', qty: '1' });
  });

  test('cambiar un renglón lleva la versión esperada en el cuerpo', async () => {
    await posApi.changeDraftLine('d-1', 'l-1', { expectedVersion: 2, qty: '1' });
    const c = llamada();
    expect(c.method).toBe('PATCH');
    expect(c.url).toMatch(/\/pos\/drafts\/d-1\/lines\/l-1$/);
    expect(c.body).toEqual({ expectedVersion: 2, qty: '1' });
  });

  test('quitar un renglón lleva la versión esperada en la query, no en un cuerpo', async () => {
    await posApi.removeDraftLine('d-1', 'l-1', 3);
    const c = llamada();
    expect(c.method).toBe('DELETE');
    expect(c.url).toMatch(/\/pos\/drafts\/d-1\/lines\/l-1\?expectedVersion=3$/);
  });

  test('la cabecera lleva su versión esperada', async () => {
    await posApi.patchDraft('d-1', { expectedHeaderVersion: 3, customerName: 'Mesa 4', discount: null });
    const c = llamada();
    expect(c.method).toBe('PATCH');
    expect(c.url).toMatch(/\/pos\/drafts\/d-1$/);
    expect(c.body).toEqual({ expectedHeaderVersion: 3, customerName: 'Mesa 4', discount: null });
  });

  test('descartar y enviar son POST con cuerpo vacío', async () => {
    await posApi.discardDraft('d-1');
    await posApi.sendDraft('d-1');
    expect(llamada(0).url).toMatch(/\/pos\/drafts\/d-1\/discard$/);
    expect(llamada(0).method).toBe('POST');
    expect(llamada(0).body).toEqual({});
    expect(llamada(1).url).toMatch(/\/pos\/drafts\/d-1\/send$/);
    expect(llamada(1).body).toEqual({});
  });

  test('subir las cuentas viejas', async () => {
    const accounts = [{ id: 't-1', folioName: 'Persa', header: {}, lines: [] }];
    await posApi.importDrafts(accounts);
    expect(llamada().url).toMatch(/\/pos\/drafts\/import$/);
    expect(llamada().body).toEqual({ accounts });
  });
});

describe('cuentas vivas', () => {
  // La fila la pide cada 30 s: sin el parámetro, el servidor mira solo 90 días. Mandarlo siempre
  // recorrería todo el histórico en cada refresco de cada tableta.
  test('sin olderDebts no se manda el parámetro', async () => {
    await posApi.liveAccounts();
    expect(llamada().url).toMatch(/\/pos\/accounts$/);
  });

  test('con olderDebts se pide explícito', async () => {
    await posApi.liveAccounts(true);
    expect(llamada().url).toMatch(/\/pos\/accounts\?olderDebts=true$/);
  });
});
