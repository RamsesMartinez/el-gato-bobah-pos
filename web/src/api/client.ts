import { useSessionStore, type SessionUser } from '../stores/session';
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
let refreshing: Promise<{ accessToken: string; user: SessionUser } | null> | null = null;

// fetchRefresh canjea la cookie de refresh por una sesión nueva pero NO la aplica al store;
// cada quien llama decide si aplicarla (así el arranque no pisa un login concurrente).
// single-flight: /auth/refresh rota y revoca el token viejo, dos en paralelo se pisarían.
function fetchRefresh(): Promise<{ accessToken: string; user: SessionUser } | null> {
  if (!refreshing) {
    refreshing = fetch(BASE + '/auth/refresh', { method: 'POST', credentials: 'include' })
      .then((r) => (r.ok ? r.json() : null))
      .catch(() => null)
      .finally(() => {
        refreshing = null;
      });
  }
  return refreshing;
}

// Reintento tras un 401: la sesión ya estaba activa, aplicamos el refresh sin condiciones.
async function tryRefresh(): Promise<boolean> {
  const s = await fetchRefresh();
  if (!s) return false;
  useSessionStore.getState().setSession(s.accessToken, s.user);
  return true;
}

// restoreSession se llama al arrancar: como el access token no se persiste, un reload en frío
// parte sin sesión y canjea la cookie HttpOnly. Solo aplica el resultado si nadie autenticó
// mientras el refresh estaba en vuelo (evita pisar un login concurrente desde /login).
export async function restoreSession(): Promise<boolean> {
  const s = await fetchRefresh();
  if (useSessionStore.getState().status !== 'loading') {
    return useSessionStore.getState().status === 'authed';
  }
  if (!s) {
    useSessionStore.getState().clear();
    return false;
  }
  useSessionStore.getState().setSession(s.accessToken, s.user);
  return true;
}

async function request<T>(method: string, path: string, body?: unknown, retry = true): Promise<T> {
  const token = useSessionStore.getState().token;
  // trazabilidad: mismo id en el header, en la consola y en logs/app.log del backend
  const requestId = uuid();
  const started = performance.now();
  const label = `${method} ${path}`;

  // FormData (subir un ticket) va tal cual y SIN Content-Type: el navegador tiene que poner el
  // suyo con el boundary del multipart. Se detecta aquí para reusar el resto de la ruta —
  // token, refresh ante 401, trazas y el sobre de error — en vez de duplicarla.
  const isForm = body instanceof FormData;
  let res: Response;
  try {
    res = await fetch(BASE + path, {
      method,
      headers: {
        ...(isForm ? {} : { 'Content-Type': 'application/json' }),
        'X-Request-Id': requestId,
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      credentials: 'include',
      body: body === undefined ? undefined : isForm ? body : JSON.stringify(body),
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
  postForm: <T>(path: string, form: FormData) => request<T>('POST', path, form),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
};
