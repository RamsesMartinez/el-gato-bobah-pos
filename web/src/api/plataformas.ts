import { api } from './client';

// Menús de plataforma (spec 020): leer lo publicado arriba y ver en qué difiere del catálogo.
//
// LA DIRECCIÓN DE LA VERDAD, y de aquí sale cómo se redacta la pantalla: **manda lo publicado en la
// plataforma**. El precio por plataforma del POS no es el precio al que se vende — es una copia que
// alguien captura DESPUÉS, a mano, para que el ticket cuadre con lo que la app ya cobró. Por eso
// una diferencia de precio significa casi siempre «el POS está desactualizado», no «la plataforma
// está mal», y la pantalla lo dice en ese orden.

/** Los tres estados de una lectura. Significan cosas opuestas y una pantalla mal hecha los muestra igual. */
export type EstadoDeLectura = 'en_curso' | 'ok' | 'fallida';

/** Por qué no sirvió. Lista cerrada: el servidor nunca manda el mensaje de la plataforma. */
export type ClaseDeFallo =
  | 'sin_credenciales'
  | 'auth_rechazada'
  | 'tiempo_agotado'
  | 'respuesta_invalida'
  | 'menu_vacio'
  | 'menu_truncado';

/** Cómo se le dice a cada fallo en pantalla. El operador no lee `failureKind`. */
export const TEXTO_DE_FALLO: Record<ClaseDeFallo, string> = {
  sin_credenciales: 'Esta tienda todavía no está conectada.',
  auth_rechazada: 'La plataforma no aceptó el acceso.',
  tiempo_agotado: 'La plataforma tardó demasiado en responder.',
  respuesta_invalida: 'La plataforma respondió algo que no se pudo leer.',
  menu_vacio: 'La plataforma respondió sin productos, así que la lectura no sirve.',
  menu_truncado: 'La plataforma entregó el menú incompleto.',
};

export interface ResumenDeLectura {
  id: number;
  status: EstadoDeLectura;
  startedAt: string;
  finishedAt: string | null;
  itemCount: number | null;
  failureKind: ClaseDeFallo | null;
  /** Lo calcula el servidor: si lo derivara la pantalla, dos tabletas con distinto reloj dirían cosas distintas. */
  stale: boolean;
}

export interface ConexionDePlataforma {
  id: number;
  platformId: number;
  platformName: string;
  externalStoreId: string;
  label: string;
  active: boolean;
  /** Si el despliegue tiene credenciales para esa plataforma. Nunca viaja el secreto ni parte de él. */
  credentialsConfigured: boolean;
  /** `null` = NUNCA se ha leído. No es lo mismo que «sin diferencias». */
  lastRead: ResumenDeLectura | null;
}

export type ClaseDeItem = 'platillo' | 'grupo' | 'opcion';
export type ClaseLocal = 'producto' | 'opcion_de_modificador';

export interface ItemDePlataforma {
  externalId: string;
  kind: ClaseDeItem;
  name: string;
  /** Centavos, como los entrega la plataforma. La conversión a pesos vive en el servidor. */
  priceCents: number;
  available: boolean;
  /** `null` = sin pareja ni propuesta. `confirmed: false` = PROPUESTA, y se pinta distinta. */
  link: {
    localId: number;
    localName: string;
    localKind: ClaseLocal;
    confirmed: boolean;
    /** Cuándo se confirmó. Es lo único que hace correcto un «deshacer el último»: la lista llega
     *  ordenada por nombre, y adivinar ahí deshace la pareja equivocada sin que nadie lo note. */
    confirmedAt?: string;
  } | null;
}

export interface Emparejamiento {
  readAt: string;
  items: ItemDePlataforma[];
  /** Lo del POS que todavía no tiene pareja: productos u opciones, según el nivel que se pidió. */
  unlinkedLocal: { id: number; name: string }[];
}

export type ClaseDeDiferencia = 'precio' | 'disponibilidad' | 'solo_en_plataforma' | 'solo_en_catalogo';

export interface Diferencia {
  kind: ClaseDeDiferencia;
  externalId?: string;
  platformName?: string;
  localId?: number;
  localName?: string;
  platformPrice?: string;
  catalogPrice?: string;
  platformAvailable?: boolean;
  catalogActive?: boolean;
}

export interface Comparacion {
  readAt: string;
  stale: boolean;
  differences: Diferencia[];
  /** Cuántos quedan sin emparejar de los dos lados. Con muchos, la lista de «solo en un lado» es ruido. */
  unpaired: number;
}

const RAIZ = '/admin/platform-menus';

export const listarConexiones = () =>
  api.get<{ connections: ConexionDePlataforma[] }>(`${RAIZ}/connections`).then((r) => r.connections);

export const crearConexion = (body: { platformId: number; externalStoreId: string; label: string }) =>
  api.post<{ id: number }>(`${RAIZ}/connections`, body);

export const borrarConexion = (id: number) => api.del<void>(`${RAIZ}/connections/${id}`);

/** Cuántas parejas se pierden al dar de baja la tienda. Se muestra ANTES de confirmar. */
export const parejasDeLaConexion = (id: number) =>
  api.get<{ links: number }>(`${RAIZ}/connections/${id}/links/count`).then((r) => r.links);

/** Dispara la lectura. Responde de inmediato: la lectura corre aparte y se sigue con `listarLecturas`. */
export const leerMenu = (id: number) =>
  api.post<{ readId: number; status: EstadoDeLectura; startedAt: string }>(`${RAIZ}/connections/${id}/read`, {});

export const listarLecturas = (id: number) =>
  api.get<{ reads: ResumenDeLectura[] }>(`${RAIZ}/connections/${id}/reads`).then((r) => r.reads);

export const emparejamiento = (id: number, kind: ClaseDeItem = 'platillo') =>
  api.get<Emparejamiento>(`${RAIZ}/connections/${id}/pairing?kind=${kind}`);

export const guardarPareja = (
  id: number,
  externalId: string,
  body: { localId: number; localKind: ClaseLocal; kind: ClaseDeItem },
) => api.put<void>(`${RAIZ}/connections/${id}/links/${encodeURIComponent(externalId)}`, body);

export const borrarPareja = (id: number, externalId: string) =>
  api.del<void>(`${RAIZ}/connections/${id}/links/${encodeURIComponent(externalId)}`);

/**
 * Las diferencias. Por omisión solo las ACCIONABLES —precio y disponibilidad—; los «solo en un
 * lado» se piden aparte, porque la mayoría nunca se van a emparejar a propósito y mezclarlos ahoga
 * todos los días lo que sí hay que corregir.
 */
export const diferencias = (id: number, kinds?: ClaseDeDiferencia[]) => {
  const qs = kinds?.length ? '?' + kinds.map((k) => `kind=${k}`).join('&') : '';
  return api.get<Comparacion>(`${RAIZ}/connections/${id}/differences${qs}`);
};
