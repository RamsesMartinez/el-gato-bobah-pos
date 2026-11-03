import { normalize } from '../utils/format';

// Nombres alternativos (spec 063): lo que comparten el editor y las fichas que lo usan.

export const MAX_ALT_NAMES = 10;
export const MAX_ALT_NAME = 60;

// SimilarAlt es un producto o artículo con nombre parecido (GET …/alt-names/similar).
export interface SimilarAlt {
  id: number;
  name: string;
  viaAlt?: string;
}

// La llave con que dos nombres son el mismo: la de domain.CatalogNameKey (sin mayúsculas, acentos ni
// espacios de más). El servidor vuelve a decidir; aquí solo se evita un viaje para lo obvio.
export const altNameKey = (s: string): string => normalize(s).replace(/\s+/g, ' ').trim();
export const cleanAltName = (s: string) => s.replace(/\s+/g, ' ').trim();

// withPendingAlt agrega a la lista lo escrito y todavía no agregado, si no sobra: la ficha lo llama al
// guardar o al salir para que no se pierda (revisión de tableta, A1). Lo repetido lo descarta en vez de
// fallar: quien escribió algo que ya está no perdió nada.
export function withPendingAlt(list: string[], draft: string, ownName: string): string[] {
  const t = cleanAltName(draft);
  if (!t || altNameKey(t) === altNameKey(ownName) || list.some((v) => altNameKey(v) === altNameKey(t))) return list;
  return [...list, t];
}

