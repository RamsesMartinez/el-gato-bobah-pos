import { describe, it, expect, vi } from 'vitest';
import { ApiError } from '../api/client';
import { createIngredientOrExisting } from './createIngredient';

// Dar de alta un insumo que ya existe con otras mayúsculas o acentos (spec 037): el servidor lo
// rechaza con 409, y quien captura —un gasto o una receta— recibe el que ya existe, con aviso, en
// lugar de un error que se pierde.
describe('createIngredientOrExisting', () => {
  const existing = [{ id: 4, name: 'Lata Coca' }, { id: 5, name: 'Leche' }];

  it('sin conflicto, crea el insumo', async () => {
    const create = vi.fn(() => Promise.resolve({ id: 9, name: 'Jarabe' }));
    const r = await createIngredientOrExisting(create, { name: 'Jarabe', baseUnitId: 5 }, existing);
    expect(r).toEqual({ ingredient: { id: 9, name: 'Jarabe' }, existed: false });
  });

  it('con un 409, devuelve el que ya se llama así sin mayúsculas ni acentos', async () => {
    const create = vi.fn(() => Promise.reject(new ApiError(409, 'CONFLICT', 'ya hay un insumo que se llama así', 'r1')));
    const r = await createIngredientOrExisting(create, { name: 'láta  coca', baseUnitId: 5 }, existing);
    expect(r).toEqual({ ingredient: { id: 4, name: 'Lata Coca' }, existed: true });
  });

  it('un 409 sin uno parecido en la lista se sigue reportando', async () => {
    const create = vi.fn(() => Promise.reject(new ApiError(409, 'CONFLICT', 'ya hay un insumo que se llama así', 'r1')));
    await expect(createIngredientOrExisting(create, { name: 'Otro', baseUnitId: 5 }, existing)).rejects.toThrow('ya hay un insumo');
  });

  it('cualquier otro error no se disfraza de duplicado', async () => {
    const create = vi.fn(() => Promise.reject(new ApiError(500, 'INTERNAL', 'falla', 'r1')));
    await expect(createIngredientOrExisting(create, { name: 'Lata Coca', baseUnitId: 5 }, existing)).rejects.toThrow('falla');
  });
});
