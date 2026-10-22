import { normalize } from '../utils/format';

// El mismo criterio que el servidor (FindArticleByName): sin mayúsculas, acentos ni espacios de más.
const comparable = (s: string) => normalize(s).trim().replace(/\s+/g, ' ');

// existingByName busca un artículo ya cargado con ese nombre: quien captura un gasto o una receta
// recibe el que existe en vez de crear otro igual (spec 037).
export function existingByName<T extends { id: number; name: string }>(name: string, list: T[]): T | undefined {
  const n = comparable(name);
  return list.find((i) => i.id > 0 && comparable(i.name) === n);
}

// Ids provisionales de los artículos que nacen dentro de una captura (spec 042, FR-003): negativos,
// para que nunca choquen con uno guardado. El servidor los crea al guardar la captura.
let temp = 0;
export function nextTempId(): number {
  temp -= 1;
  return temp;
}
