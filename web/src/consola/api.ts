// El cliente HTTP de la consola: propio, no el del POS.
//
// No es duplicación por descuido. El del POS trae reintento con refresh, cola de peticiones,
// manejo de errores de cobro y el store de sesión del negocio — todo lo que una consola de dos
// pantallas no necesita, y todo lo que la ataría al ciclo de vida del POS. Son dos productos.

// En desarrollo, el servidor de Vite hace proxy de /api al backend (mismo origen, sin CORS). En
// producción la consola vive en staff.… y la API en api.…, así que el build recibe VITE_API_URL.
const BASE = import.meta.env.VITE_API_URL || '/api/v1';

export class ErrorDeConsola extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

// El acceso vive SOLO en memoria, igual que en el POS: lo que no se guarda no lo lee un XSS.
// Recargar la pestaña pide entrar otra vez, y para una consola que abre una persona en su
// computadora eso no le cuesta nada a nadie — a cambio no hay una sesión de plataforma durmiendo
// en el disco de un navegador.
let acceso = '';

export function hayAcceso(): boolean {
  return acceso !== '';
}

export function olvidarAcceso(): void {
  acceso = '';
}

// MENSAJE_SIN_RED: lo que se dice cuando el `fetch` ni siquiera llegó al servidor.
//
// Sin esto, la pantalla pinta el texto que escribe el navegador —"Failed to fetch" en Chrome,
// "NetworkError when attempting to fetch resource." en Firefox—: inglés, interno del motor y sin
// nada que hacer al respecto. El fallo de red es el más común de todos y es el que peor se veía.
export const MENSAJE_SIN_RED = 'No se pudo conectar. Revisa la conexión e intenta de nuevo.';

async function pedir<T>(ruta: string, init?: RequestInit): Promise<T> {
  const res = await fetchCurado(BASE + ruta, {
    ...init,
    headers: {
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...(acceso ? { Authorization: 'Bearer ' + acceso } : {}),
      ...init?.headers,
    },
  });
  if (!res.ok) {
    let mensaje = 'No se pudo completar la operación.';
    try {
      const cuerpo = await res.json();
      if (cuerpo?.error?.message) mensaje = cuerpo.error.message;
    } catch {
      // Respuesta sin cuerpo JSON (502 de un proxy, por ejemplo): queda el mensaje genérico.
    }
    throw new ErrorDeConsola(res.status, mensaje);
  }
  return res.json() as Promise<T>;
}

// fetchCurado traduce el rechazo del navegador —red caída, DNS, CORS— a un error nuestro. Va
// aquí y no en cada pantalla para que ninguna vuelva a enseñar el texto del motor.
async function fetchCurado(url: string, init: RequestInit): Promise<Response> {
  try {
    return await fetch(url, init);
  } catch {
    // El 0 dice "ni siquiera hubo respuesta", y lo distingue de un 401 o un 500 para quien lo lea.
    throw new ErrorDeConsola(0, MENSAJE_SIN_RED);
  }
}

export interface Operador {
  id: number;
  username: string;
  name: string;
}

export async function entrar(username: string, password: string): Promise<Operador> {
  const s = await pedir<{ accessToken: string; operator: Operador }>('/platform/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  });
  acceso = s.accessToken;
  return s.operator;
}

export interface Empresa {
  id: number;
  slug: string;
  name: string;
  activa: boolean;
  createdAt: string;
}

export interface Empresas {
  items: Empresa[];
  schema: { version: number };
}

export function empresas(): Promise<Empresas> {
  return pedir<Empresas>('/platform/companies');
}

export interface AccionDeUso {
  accion: string;
  veces: number;
}

export interface UsoPorRol {
  // `null` = sin corte: ese uso existe pero atribuirlo a un rol identificaría a una persona.
  rol: string | null;
  veces: number;
}

export interface PantallaDeUso {
  pantalla: string;
  aperturas: number;
  acciones: AccionDeUso[];
  porRol: UsoPorRol[];
}

export interface MapaDeUso {
  periodo: { desde: string; hasta: string };
  pantallas: PantallaDeUso[];
}

// mapaDeUso pide el uso de un periodo. `empresa` ausente = todas juntas.
export function mapaDeUso(desde: string, hasta: string, empresa?: number): Promise<MapaDeUso> {
  const q = new URLSearchParams({ desde, hasta });
  if (empresa) q.set("empresa", String(empresa));
  return pedir<MapaDeUso>("/platform/usage?" + q.toString());
}
