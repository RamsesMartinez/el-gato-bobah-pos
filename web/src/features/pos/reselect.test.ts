import { expect, test } from 'vitest';
import type { Menu } from '../../types/pos';
import { productToReselect } from './reselect';

// Tras un rechazo por menú viejo la hoja de extras se reabre con el menú NUEVO. Reabrirla con un
// producto que ya no se vende, o una hoja vacía para uno sin extras, sería otro callejón sin salida.
const menu = {
  products: [
    { id: 630, name: 'Frappé', groups: [{ id: 160, options: [] }] },
    { id: 41, name: 'Taro', groups: [] },
  ],
} as unknown as Menu;

test('reabre el producto con sus extras actuales', () => {
  expect(productToReselect(menu, 630)?.groups[0].id).toBe(160);
});

test('un producto que ya no está en el menú no abre nada', () => {
  expect(productToReselect(menu, 999)).toBeNull();
});

test('un producto sin extras no abre una hoja vacía', () => {
  expect(productToReselect(menu, 41)).toBeNull();
});

test('sin menú no abre nada', () => {
  expect(productToReselect(undefined, 630)).toBeNull();
});
