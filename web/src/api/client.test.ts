import { vi } from 'vitest';
import { ApiError, api } from './client';

// El backend manda el motivo del fallo como DATO (`error.details`), no solo como prosa. El cliente
// lo tiene que dejar pasar tal cual: si se pierde aquí, la pantalla vuelve a tener que adivinar el
// producto escarbando en el texto del mensaje.
function respondeCon(status: number, body: unknown) {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })));
}

afterEach(() => { vi.unstubAllGlobals(); });

test('el detalle del producto llega intacto al ApiError', async () => {
  respondeCon(422, {
    error: {
      code: 'UNPROCESSABLE',
      message: 'producto no disponible: Chococino',
      details: { productId: 510, productName: 'Chococino' },
    },
  });

  const err: unknown = await api.post('/orders', {}).catch((e) => e);
  if (!(err instanceof ApiError)) throw new Error('se esperaba un ApiError');
  expect(err).toBeInstanceOf(ApiError);
  expect(err.code).toBe('UNPROCESSABLE');
  expect(err.details?.productId).toBe(510);
  expect(err.details?.productName).toBe('Chococino');
});

test('un error sin detalles deja details en undefined', async () => {
  respondeCon(404, { error: { code: 'NOT_FOUND', message: 'no encontrado' } });

  const err: unknown = await api.get('/orders/9').catch((e) => e);
  if (!(err instanceof ApiError)) throw new Error('se esperaba un ApiError');
  expect(err).toBeInstanceOf(ApiError);
  expect(err.details).toBeUndefined();
});
