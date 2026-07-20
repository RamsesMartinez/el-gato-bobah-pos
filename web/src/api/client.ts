import { useSessionStore } from '../stores/session';
import { uuid } from '../utils/uuid';

// En dev, Vite hace proxy de /api → localhost:8080. En prod, mismo origen tras Caddy.
const BASE = import.meta.env.VITE_API_URL || '/api/v1';
const DEBUG = import.meta.env.DEV;

export class ApiError extends Error {
  code: string;
  status: number;
  requestId: string;
  constructor(status: number, code: string, message: string, requestId: string) {
    super(message);
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

// El access token dura 15 min; el refresh (cookie HttpOnly) dura 30 días. Ante un 401
// canjeamos el refresh por un access nuevo y reintentamos, en vez de expulsar al usuario.
// single-flight: /auth/refresh rota y revoca el token viejo, así que dos refresh en
// paralelo se pisarían → compartimos una sola promesa entre todas las peticiones que fallen.
let refreshing: Promise<boolean> | null = null;

async function tryRefresh(): Promise<boolean> {
  if (!refreshing) {
    refreshing = fetch(BASE + '/auth/refresh', { method: 'POST', credentials: 'include' })
      .then(async (r) => {
        if (!r.ok) return false;
        const { accessToken, user } = await r.json();
        useSessionStore.getState().setSession(accessToken, user);
        return true;
      })
      .catch(() => false)
      // El clear() en fallo lo hace quien llama (request() ante un 401, restoreSession al
      // arrancar): así el refresh de arranque no puede pisar un login que ocurrió mientras
      // seguía en vuelo.
      .finally(() => {
        refreshing = null;
      });
  }
  return refreshing;
}

// restoreSession se llama al arrancar la app: como el access token ya no se persiste, un
// reload en frío parte sin sesión y hay que canjear la cookie HttpOnly de refresh. Comparte
// el single-flight con los reintentos de 401. Solo degrada a 'anon' si nadie autenticó
// mientras tanto (evita el clobber de un login concurrente desde /login).
export async function restoreSession(): Promise<boolean> {
  const ok = await tryRefresh();
  if (!ok && useSessionStore.getState().status === 'loading') {
    useSessionStore.getState().clear();
  }
  return ok;
}

async function request<T>(method: string, path: string, body?: unknown, retry = true): Promise<T> {
  const token = useSessionStore.getState().token;
  // trazabilidad: mismo id en el header, en la consola y en logs/app.log del backend
  const requestId = uuid();
  const started = performance.now();
  const label = `${method} ${path}`;

  let res: Response;
  try {
    res = await fetch(BASE + path, {
      method,
      headers: {
        'Content-Type': 'application/json',
        'X-Request-Id': requestId,
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      credentials: 'include',
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (err) {
    const ms = Math.round(performance.now() - started);
    console.error(`[api] ✗ ${label} · red caída · ${ms}ms · id=${requestId}`, err);
    throw new ApiError(0, 'NETWORK', 'Sin conexión con el servidor', requestId);
  }

  const ms = Math.round(performance.now() - started);
  // el backend devuelve el mismo id (o el que generó si no lo mandamos)
  const traceId = res.headers.get('X-Request-Id') || requestId;

  if (res.status === 401) {
    // No intentamos refrescar sobre los propios endpoints de auth (un 401 ahí es real:
    // credenciales malas o refresh vencido) ni si ya reintentamos una vez.
    if (retry && !path.startsWith('/auth/') && (await tryRefresh())) {
      return request<T>(method, path, body, false);
    }
    useSessionStore.getState().clear();
  }
  if (!res.ok) {
    let code = 'ERROR';
    let message = `Error ${res.status}`;
    try {
      const data = await res.json();
      code = data?.error?.code ?? code;
      message = data?.error?.message ?? message;
    } catch {
      /* respuesta sin cuerpo JSON */
    }
    console.error(`[api] ✗ ${label} · ${res.status} ${code} · ${ms}ms · id=${traceId}`);
    throw new ApiError(res.status, code, message, traceId);
  }

  if (DEBUG) {
    console.debug(`[api] ✓ ${label} · ${res.status} · ${ms}ms · id=${traceId}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
};
