import { useSessionStore, type SessionUser } from '../stores/session';
import { deviceHeaders } from './origin';
import { uuid } from '../utils/uuid';

// En dev, Vite hace proxy de /api → localhost:8080. En prod, mismo origen tras Caddy.
const BASE = import.meta.env.VITE_API_URL || '/api/v1';
const DEBUG = import.meta.env.DEV;

// ApiErrorDetails es el motivo del fallo como DATO, tal como lo manda el backend. Existe para que
// la pantalla no tenga que deducir nada del texto del mensaje: el servidor decide qué producto
// tumbó el cobro y el front nada más lo pinta. `productName` puede venir vacío — el backend solo
// nombra lo que el catálogo de la empresa conoce.
export interface ApiErrorDetails {
  productId?: number;
  productName?: string;
  // El renglón de la cuenta que el servidor rechazó por menú viejo (MENU_CHANGED al mandar o cobrar).
  lineId?: string;
  // El motivo de merma que ya existe con ese nombre (spec 039): la pantalla lo elige en vez de crear otro.
  existingId?: number;
  // El artículo con ese nombre está archivado (spec 042): la pantalla ofrece reactivarlo.
  archived?: boolean;
  // El artículo sin densidad de una compra entre peso y líquido (spec 058): la pantalla ofrece capturarla.
  ingredientId?: number;
  // El nombre alternativo que chocó (spec 063, ALT_NAME_TAKEN); el mensaje ya lo nombra.
  name?: string;
}

export class ApiError extends Error {
  code: string;
  status: number;
  requestId: string;
  details?: ApiErrorDetails;
  // Segundos que pide esperar un 429 (header Retry-After), para decir en cuánto reintentar.
  retryAfter?: number;
  constructor(status: number, code: string, message: string, requestId: string, details?: ApiErrorDetails, retryAfter?: number) {
    super(message);
    this.status = status;
    this.code = code;
    this.requestId = requestId;
    this.details = details;
    this.retryAfter = retryAfter;
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

// Lo que /auth/me dice de la sesión viva (spec 068).
interface MeResponse {
  id: number;
  companyId: number;
  name: string;
  role: string;
  roleId: number;
  roleName: string;
  permissions: string[];
}

let refreshingPermissions: Promise<void> | null = null;

// refreshPermissions vuelve a pedir a /auth/me lo que puede la sesión (FR-018): la administradora
// cambia un rol y el servidor lo aplica en la siguiente petición, pero la pantalla seguiría
// ofreciendo lo de antes hasta volver a entrar. Una sola petición en vuelo aunque lleguen varios 403.
// Solo se aplica si sigue siendo la misma persona: un desbloqueo con el PIN de otra mientras estaba
// en vuelo no hereda los permisos de quien se fue. A quien entró se le pide una vez más al terminar:
// su 403 reutilizó la promesa de la anterior y, sin eso, se quedaría con los permisos con que entró.
export function refreshPermissions(): Promise<void> {
  if (refreshingPermissions) return refreshingPermissions;
  refreshingPermissions = fetchPermissions(true).finally(() => { refreshingPermissions = null; });
  return refreshingPermissions;
}

async function fetchPermissions(retryIfSwitched: boolean): Promise<void> {
  const { user, token } = useSessionStore.getState();
  if (!user || !token) return;
  let me: MeResponse;
  try {
    me = await request<MeResponse>('GET', '/auth/me');
  } catch {
    return; // sin red o sin sesión: se queda lo que había y el servidor sigue siendo la barrera
  }
  const current = useSessionStore.getState().user;
  if (!current) return;
  if (current.id === me.id && current.companyId === me.companyId) {
    useSessionStore.setState({
      user: { ...current, name: me.name, role: me.role, roleId: me.roleId, roleName: me.roleName, permissions: me.permissions },
    });
    return;
  }
  const switched = current.id !== user.id || current.companyId !== user.companyId;
  if (retryIfSwitched && switched) await fetchPermissions(false);
}

// watchPermissions refresca los permisos cada vez que la pestaña vuelve al frente: una tableta que
// pasó la noche suspendida trae los de ayer. Devuelve con qué dejar de escuchar.
export function watchPermissions(): () => void {
  const onVisible = () => {
    if (document.visibilityState === 'visible' && useSessionStore.getState().status === 'authed') void refreshPermissions();
  };
  document.addEventListener('visibilitychange', onVisible);
  return () => document.removeEventListener('visibilitychange', onVisible);
}

// raw devuelve el Response sin parsear: no toda respuesta es JSON (el logo del ticket es un binario).
// Se resuelve aquí y no con un fetch aparte para no abrir una segunda ruta que se olvide del token,
// del refresh ante 401, de la traza o del sobre de error.
// `extra` son encabezados propios de una petición; hoy solo If-None-Match del menú.
async function request<T>(
  method: string, path: string, body?: unknown, retry = true, raw = false, extra?: Record<string, string>,
): Promise<T> {
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
        ...deviceHeaders(),
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...extra,
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

  if (res.status === 401 && retry && (!path.startsWith('/auth/') || path === '/auth/me') && (await tryRefresh())) {
    // No se reintenta sobre los propios endpoints de auth (un 401 ahí es real: credenciales malas
    // o refresh vencido) ni si ya se reintentó una vez. /auth/me sí: es una lectura de la sesión, y
    // al volver al frente con el token de 15 min vencido cerraría un turno que sigue vivo.
    return request<T>(method, path, body, false, raw, extra);
  }
  // 304 no es un error: es «lo que ya tienes sigue vigente», y solo llega si se pidió condicional.
  if (!res.ok && res.status !== 304) {
    // El cuerpo se lee UNA sola vez y antes de decidir qué hacer con el 401: el código del error es
    // lo que distingue un 401 de otro, y `res.json()` no se puede volver a leer.
    let code = 'ERROR';
    let message = `Error ${res.status}`;
    let details: ApiErrorDetails | undefined;
    try {
      const data = await res.json();
      code = data?.error?.code ?? code;
      message = data?.error?.message ?? message;
      details = data?.error?.details ?? undefined;
    } catch {
      /* respuesta sin cuerpo JSON */
    }
    // "Ese PIN no es" NO es "se acabó el turno". Los dos llegan como 401, y barrerlos juntos hacía
    // que un dedazo en la pantalla de bloqueo mandara al operador a escribir usuario y contraseña
    // — el toque que esa pantalla viene justo a quitar, y con prisa eso enseña a no bloquear la
    // tableta.
    if (res.status === 401 && code !== 'INVALID_CREDENTIALS') {
      // La sesión terminó. Se dice POR QUÉ para que la pantalla de login no muestre un error
      // genérico — quien llega en la mañana con la tableta caducada vería "no autorizado" y
      // creería que algo se rompió.
      useSessionStore.getState().clear('caducada');
    }
    // Un 403 puede ser que el rol cambió: se traen los permisos de nuevo para que la pantalla deje
    // de ofrecer lo que ya no puede. Nunca por un 403 del propio /auth/me, o no terminaría.
    if (res.status === 403 && path !== '/auth/me') void refreshPermissions();
    console.error(`[api] ✗ ${label} · ${res.status} ${code} · ${ms}ms · id=${traceId}`);
    const retryAfter = Number(res.headers.get('Retry-After')) || undefined;
    throw new ApiError(res.status, code, message, traceId, details, retryAfter);
  }

  if (DEBUG) {
    console.debug(`[api] ✓ ${label} · ${res.status} · ${ms}ms · id=${traceId}`);
  }
  if (raw) return res as T;
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  getRaw: (path: string) => request<Response>('GET', path, undefined, true, true),
  // GET condicional: con el ETag de lo que ya se tiene, un 304 dice que sigue vigente sin cuerpo.
  getConditional: async <T>(path: string, etag: string | null): Promise<{ notModified: true } | { notModified: false; data: T; etag: string | null }> => {
    const res = await request<Response>('GET', path, undefined, true, true, etag ? { 'If-None-Match': etag } : undefined);
    if (res.status === 304) return { notModified: true };
    return { notModified: false, data: (await res.json()) as T, etag: res.headers.get('ETag') };
  },
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  postForm: <T>(path: string, form: FormData) => request<T>('POST', path, form),
  putForm: <T>(path: string, form: FormData) => request<T>('PUT', path, form),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
};
