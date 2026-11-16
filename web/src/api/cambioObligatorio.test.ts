import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { useSessionStore } from '../stores/session';
import { api, notePasswordChanged, refreshPermissions } from './client';

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

  test('/auth/me sin el indicador lo apaga: es el camino de vuelta si quedó encendido de más', async () => {
    useSessionStore.setState({ token: 't', status: 'authed', user: { ...usuario, mustChangePassword: true } });
    rutas['/auth/me'] = () => json(200, { ...usuario, mustChangePassword: false });
    await refreshPermissions();
    expect(useSessionStore.getState().user?.mustChangePassword).toBe(false);
  });

  // Un admin restablece a Ana con la tableta bloqueada; Beto desbloquea con su PIN y las peticiones
  // que salieron con el token de Ana vuelven con el 403. A Beto no se le pide cambiar nada.
  test('el 403 de una petición de la persona anterior no le pide el cambio a quien entró', async () => {
    useSessionStore.setState({ token: 'token-ana', status: 'authed', user: { ...usuario } });
    let soltar!: (r: Response) => void;
    rutas['/company'] = () => undefined as unknown as Response;
    vi.mocked(fetch).mockImplementationOnce(() => new Promise<Response>((r) => { soltar = r; }));
    rutas['/auth/me'] = () => json(200, { ...usuario, id: 8, name: 'Beto' });
    const enVuelo = api.get('/company').catch(() => {});
    useSessionStore.setState({ token: 'token-beto', user: { ...usuario, id: 8, name: 'Beto' } });
    soltar(json(403, { error: { code: 'PASSWORD_CHANGE_REQUIRED', message: 'x' } }));
    await enVuelo;
    expect(useSessionStore.getState().user?.mustChangePassword).not.toBe(true);
  });

  // La pestaña vuelve al frente, /auth/me tarda, la persona cambia su contraseña y la respuesta vieja
  // llega después: no vuelve a pedir el cambio que ya se hizo.
  test('un /auth/me que salió antes del cambio no vuelve a encender el indicador', async () => {
    useSessionStore.setState({ token: 't', status: 'authed', user: { ...usuario, mustChangePassword: true } });
    let soltar!: (r: Response) => void;
    vi.mocked(fetch).mockImplementationOnce(() => new Promise<Response>((r) => { soltar = r; }));
    const enVuelo = refreshPermissions();
    await Promise.resolve();
    notePasswordChanged();
    useSessionStore.setState({ user: { ...usuario, mustChangePassword: false } });
    soltar(json(200, { ...usuario, mustChangePassword: true }));
    await enVuelo;
    expect(useSessionStore.getState().user?.mustChangePassword).toBe(false);
  });
});
