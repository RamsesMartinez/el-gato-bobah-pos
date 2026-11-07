import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { useSessionStore } from '../stores/session';
import { api, refreshPermissions, watchPermissions } from './client';

// FR-018 (spec 068): cuando la administradora cambia lo que puede un rol, el servidor lo aplica en la
// siguiente petición, pero la pantalla seguiría ofreciendo (o escondiendo) lo de antes hasta volver a
// entrar. Se vuelve a pedir /auth/me al volver la pestaña al frente y al recibir un 403.

const me = (over: Record<string, unknown> = {}) => ({
  id: 7, companyId: 2, name: 'Ana', role: '', roleId: 40, roleName: 'Encargada', permissions: ['cash.operate', 'sales.view'], ...over,
});

let rutas: Record<string, () => Response>;
const llamadas = () => vi.mocked(fetch).mock.calls.map(([url]) => String(url));
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

beforeEach(() => {
  rutas = {
    '/auth/me': () => json(200, me()),
    '/sales': () => json(403, { error: { code: 'FORBIDDEN', message: 'sin permiso' } }),
  };
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    const ruta = Object.keys(rutas).find((r) => url.endsWith(r));
    return ruta ? rutas[ruta]() : json(404, {});
  }));
  useSessionStore.setState({
    token: 't', status: 'authed',
    user: { id: 7, companyId: 2, companySlug: 'gatobobah', name: 'Ana', role: 'cajero', roleId: 3, roleName: 'Cajero', permissions: ['cash.operate'] },
  });
});
afterEach(() => {
  vi.unstubAllGlobals();
  useSessionStore.setState({ token: null, user: null, status: 'anon' });
});

const esperarMe = (n = 1) => vi.waitFor(() => expect(llamadas().filter((u) => u.endsWith('/auth/me'))).toHaveLength(n));

describe('los permisos de la sesión se refrescan', () => {
  test('un 403 vuelve a pedir /auth/me y la sesión toma los permisos y el rol nuevos', async () => {
    await api.get('/sales').catch(() => {});
    await esperarMe();
    await vi.waitFor(() => expect(useSessionStore.getState().user?.permissions).toEqual(['cash.operate', 'sales.view']));
    const user = useSessionStore.getState().user!;
    expect(user).toMatchObject({ role: '', roleId: 40, roleName: 'Encargada', companySlug: 'gatobobah' });
    // No es un login: el token se queda como estaba.
    expect(useSessionStore.getState().token).toBe('t');
  });

  // EL TOKEN VENCIDO NO CIERRA LA SESIÓN. Al volver la pestaña después de 15 minutos el token ya
  // venció: /auth/me responde 401 y, sin renovar, el cliente cerraba la sesión con el turno vivo.
  test('un 401 de /auth/me renueva el token y no cierra la sesión', async () => {
    let veces = 0;
    rutas['/auth/me'] = () => (++veces === 1 ? json(401, { error: { code: 'UNAUTHORIZED', message: 'x' } }) : json(200, me()));
    rutas['/auth/refresh'] = () => json(200, { accessToken: 't2', user: { ...me(), companySlug: 'gatobobah' } });
    await refreshPermissions();
    await vi.waitFor(() => expect(useSessionStore.getState().user?.permissions).toEqual(['cash.operate', 'sales.view']));
    expect(useSessionStore.getState().status).toBe('authed');
    expect(useSessionStore.getState().token).toBe('t2');
  });

  // Sin esto, un 403 de /auth/me pediría /auth/me otra vez, y así sin fin.
  test('un 403 de /auth/me no vuelve a pedir /auth/me', async () => {
    rutas['/auth/me'] = () => json(403, { error: { code: 'FORBIDDEN', message: 'x' } });
    await refreshPermissions();
    await new Promise((r) => setTimeout(r, 10));
    expect(llamadas().filter((u) => u.endsWith('/auth/me'))).toHaveLength(1);
  });

  test('dos 403 a la vez piden /auth/me una sola vez', async () => {
    await Promise.all([api.get('/sales').catch(() => {}), api.get('/sales').catch(() => {})]);
    await esperarMe();
    await new Promise((r) => setTimeout(r, 10));
    expect(llamadas().filter((u) => u.endsWith('/auth/me'))).toHaveLength(1);
  });

  // Desbloquear con el PIN de otra persona cambia el usuario mientras /auth/me estaba en vuelo: la
  // respuesta es de quien ya no está y no se le pega a quien entró.
  test('la respuesta de otro usuario no se aplica', async () => {
    rutas['/auth/me'] = () => {
      useSessionStore.setState({ user: { id: 9, companyId: 2, name: 'Beto', role: 'mesero', permissions: [] } });
      return json(200, me());
    };
    await refreshPermissions();
    expect(useSessionStore.getState().user).toMatchObject({ id: 9, permissions: [] });
  });

  test('sin sesión no pide nada', async () => {
    useSessionStore.setState({ token: null, user: null, status: 'anon' });
    await refreshPermissions();
    expect(llamadas()).toEqual([]);
  });

  test('al volver la pestaña al frente se piden; al irse, no', async () => {
    const dejar = watchPermissions();
    const visibilidad = vi.spyOn(document, 'visibilityState', 'get');
    try {
      visibilidad.mockReturnValue('hidden');
      document.dispatchEvent(new Event('visibilitychange'));
      expect(llamadas()).toEqual([]);
      visibilidad.mockReturnValue('visible');
      document.dispatchEvent(new Event('visibilitychange'));
      await esperarMe();
      await vi.waitFor(() => expect(useSessionStore.getState().user?.permissions).toContain('sales.view'));
    } finally {
      dejar();
      visibilidad.mockRestore();
    }
    // Ya sin escuchar, volver al frente no pide nada.
    document.dispatchEvent(new Event('visibilitychange'));
    await new Promise((r) => setTimeout(r, 10));
    expect(llamadas().filter((u) => u.endsWith('/auth/me'))).toHaveLength(1);
  });
});
