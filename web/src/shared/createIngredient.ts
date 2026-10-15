import { ApiError } from '../api/client';
import { normalize } from '../utils/format';

// El mismo criterio que el servidor (IngredientNameTaken): sin mayúsculas, acentos ni espacios de más.
const comparable = (s: string) => normalize(s).trim().replace(/\s+/g, ' ');

// createIngredientOrExisting da de alta un insumo y, si ya hay uno que se llama así (409, spec 037),
// devuelve ese: quien captura un gasto o una receta lo recibe con aviso en lugar de un error que se
// pierde. Cualquier otro error se sigue reportando tal cual.
export async function createIngredientOrExisting<T extends { id: number; name: string }>(
  create: (b: { name: string; baseUnitId: number }) => Promise<T>,
  body: { name: string; baseUnitId: number },
  existing: T[],
): Promise<{ ingredient: T; existed: boolean }> {
  try {
    return { ingredient: await create(body), existed: false };
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) {
      const found = existing.find((i) => comparable(i.name) === comparable(body.name));
      if (found) return { ingredient: found, existed: true };
    }
    throw e;
  }
}
