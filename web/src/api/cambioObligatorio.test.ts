import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { useSessionStore } from '../stores/session';
import { api, refreshPermissions, restoreSession } from './client';

// El cambio obligatorio de contraseña (#311) se evadía recargando: el desvío vivía en la memoria del
// login. La pantalla ahora lo toma de lo que dice el servidor, venga de donde venga.

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const usuario = { id: 7, companyId: 2, companySlug: 'gatobobah', name: 'Ana', role: 'cajero', roleId: 3, roleName: 'Cajero', permissions: ['cash.operate'] };
let rutas: Record<string, () => Response>;
const llamadas = () => vi.mocked(fetch).mock.calls.map(([url]) => String(url));

beforeEach(() => {
  rutas = {};
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    const ruta = Object.keys(rutas).find((r) => url.endsWith(r));
    return ruta ? rutas[ruta]() : json(404, {});
  }));
});
afterEach(() => {
  vi.unstubAllGlobals();
  useSessionStore.setState({ token: null, user: null, status: 'anon' });
});

describe('el cambio obligatorio sale del servidor', () => {
  test('al recargar, la sesión que devuelve el refresh trae el indicador', async () => {
    useSessionStore.setState({ token: null, user: null, status: 'loading' });
    rutas['/auth/refresh'] = () => json(200, { accessToken: 't', user: { ...usuario, mustChangePassword: true } });
    await restoreSession();
    expect(useSessionStore.getState().user?.mustChangePassword).toBe(true);
  });

  test('un 403 PASSWORD_CHANGE_REQUIRED enciende el indicador y no vuelve a pedir permisos', async () => {
    useSessionStore.setState({ token: 't', status: 'authed', user: { ...usuario } });
    rutas['/company'] = () => json(403, { error: { code: 'PASSWORD_CHANGE_REQUIRED', message: 'cambia tu contraseña para continuar' } });
    await api.get('/company').catch(() => {});
    expect(useSessionStore.getState().user?.mustChangePassword).toBe(true);
    await new Promise((r) => setTimeout(r, 10));
    expect(llamadas().filter((u) => u.endsWith('/auth/me'))).toHaveLength(0);
  });

  test('/auth/me trae el indicador: un restablecimiento con la sesión abierta se ve al volver al frente', async () => {
    useSessionStore.setState({ token: 't', status: 'authed', user: { ...usuario } });
    rutas['/auth/me'] = () => json(200, { ...usuario, mustChangePassword: true });
    await refreshPermissions();
    expect(useSessionStore.getState().user?.mustChangePassword).toBe(true);
  });
});
