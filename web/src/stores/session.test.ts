import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { registrarLimpiezaDeTenant, useSessionStore } from './session';
import { restoreSession } from '../api/client';

// A02/A05: el access token vive solo en memoria. Un reload en frío parte sin token y debe
// re-autenticar canjeando la cookie HttpOnly de refresh, sin obligar a re-loguearse.

beforeEach(() => {
  useSessionStore.setState({ token: null, user: null, status: 'loading' });
});
afterEach(() => vi.restoreAllMocks());

test('el token no se persiste en localStorage', () => {
  useSessionStore.getState().setSession('secreto', { id: 1, companyId: 1, name: 'Kate', role: 'cajero' });
  const dumped = JSON.stringify(localStorage);
  expect(dumped).not.toContain('secreto');
});

test('reload en frío: canjea la cookie de refresh por una sesión nueva', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () =>
      new Response(
        JSON.stringify({ accessToken: 'tok-nuevo', user: { id: 1, companyId: 1, name: 'Kate', role: 'cajero' } }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    ),
  );
  const ok = await restoreSession();
  expect(ok).toBe(true);
  const s = useSessionStore.getState();
  expect(s.token).toBe('tok-nuevo');
  expect(s.user?.name).toBe('Kate');
  expect(s.status).toBe('authed');
});

test('el refresh de arranque NO pisa un login concurrente', async () => {
  // Un login (desde /login) estableció la sesión B mientras el refresh de arranque —con la
  // cookie de otro operador A— seguía en vuelo. Al resolver, NO debe sobreescribir a B.
  useSessionStore.getState().setSession('tok-B', { id: 2, companyId: 1, name: 'Beto', role: 'mesero' });
  vi.stubGlobal(
    'fetch',
    vi.fn(async () =>
      new Response(
        JSON.stringify({ accessToken: 'tok-A', user: { id: 1, companyId: 1, name: 'Ana', role: 'admin' } }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    ),
  );
  await restoreSession();
  const s = useSessionStore.getState();
  expect(s.token).toBe('tok-B');
  expect(s.user?.name).toBe('Beto');
});

test('reload en frío sin cookie válida: queda anónimo', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response('', { status: 401 })));
  const ok = await restoreSession();
  expect(ok).toBe(false);
  const s = useSessionStore.getState();
  expect(s.token).toBeNull();
  expect(s.status).toBe('anon');
});

// Entrar con otra empresa tiene que tirar lo que quedó del tenant anterior. Es la barrera que
// impide que el POS pinte un producto ajeno: el backend lo rechaza, pero el operador se enteraría
// hasta el cobro, con el ticket armado y el cliente enfrente.
test('entrar con otra empresa dispara la limpieza; volver a la misma no', () => {
  localStorage.clear();
  const limpiar = vi.fn();
  registrarLimpiezaDeTenant(limpiar);

  useSessionStore.getState().setSession('tok', { id: 1, companyId: 1, name: 'Ana', role: 'admin' });
  expect(limpiar).toHaveBeenCalledTimes(1); // dispositivo sin marca: se limpia por si acaso

  useSessionStore.getState().setSession('tok', { id: 4, companyId: 1, name: 'Beto', role: 'cajero' });
  expect(limpiar).toHaveBeenCalledTimes(1); // misma empresa, otro usuario: no se toca nada

  useSessionStore.getState().setSession('tok', { id: 9, companyId: 2, name: 'Cris', role: 'admin' });
  expect(limpiar).toHaveBeenCalledTimes(2); // otra empresa: se limpia

  registrarLimpiezaDeTenant(() => {});
});
