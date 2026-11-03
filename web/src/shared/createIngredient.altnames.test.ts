import { existingByName } from './createIngredient';

// Spec 063 (FR-014, revisión del front): crear «leche lala» cuando ya es otro nombre de «Leche entera»
// usa ésa; el servidor respondía NAME_TAKEN y la receta entera no se guardaba.
test('encuentra por otro nombre, sin mayúsculas, acentos ni espacios de más', () => {
  const list = [{ id: 1, name: 'Leche entera', altNames: ['Leche Lala'] }, { id: 2, name: 'Azúcar' }];
  expect(existingByName('leche  LALA', list)?.id).toBe(1);
  expect(existingByName('azucar', list)?.id).toBe(2);
  expect(existingByName('Leche deslactosada', list)).toBeUndefined();
});
