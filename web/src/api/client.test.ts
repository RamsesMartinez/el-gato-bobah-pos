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

// UN DEDAZO EN EL PIN NO PUEDE TIRAR LA SESIÓN DE LA ESTACIÓN.
//
// El 401 de la pantalla de bloqueo es "ese PIN no es", no "se acabó el turno". El cliente barría
// los dos con el mismo `clear('caducada')`, así que teclear un dígito de más mandaba al operador a
// escribir usuario y contraseña — el toque que la pantalla de bloqueo viene justo a quitar. Y con
// prisa eso enseña a no bloquear la tableta.
test('un PIN incorrecto no cierra la sesión de la estación', async () => {
  const { useSessionStore } = await import('../stores/session');
  useSessionStore.getState().setSession('token-vivo', {
    id: 7, name: 'Ana', role: 'cajero', companyId: 1, companyName: 'Gato',
  } as never);

  respondeCon(401, { error: { code: 'INVALID_CREDENTIALS', message: 'credenciales inválidas' } });
  await api.post('/auth/pin-switch', { userId: 9, pin: '0000' }).catch(() => {});

  expect(useSessionStore.getState().token).toBe('token-vivo');
});

// Pero el turno vencido SÍ la cierra, y con el motivo puesto: la pantalla de login tiene que poder
// decir "terminó el turno" en vez de un "no autorizado" que se lee como algo roto.
test('un turno vencido sí cierra la sesión', async () => {
  const { useSessionStore } = await import('../stores/session');
  useSessionStore.getState().setSession('token-vivo', {
    id: 7, name: 'Ana', role: 'cajero', companyId: 1, companyName: 'Gato',
  } as never);

  respondeCon(401, { error: { code: 'UNAUTHORIZED', message: 'no autenticado' } });
  await api.post('/auth/pin-switch', { userId: 9, pin: '0000' }).catch(() => {});

  expect(useSessionStore.getState().token).toBeNull();
});
