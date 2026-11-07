import { describe, expect, test } from 'vitest';
import { groupByArea, permissionDiff, productDefault, adjustedText } from './roleEdit';

describe('permissionDiff', () => {
  // Lo que se manda es lo tocado: marcar y volver a desmarcar no es un cambio.
  test('solo lo que difiere de lo que la hoja mostró al abrir', () => {
    expect(permissionDiff(['a', 'b'], new Set(['b', 'c']))).toEqual({ add: ['c'], remove: ['a'] });
    expect(permissionDiff(['a'], new Set(['a']))).toEqual({ add: [], remove: [] });
  });
});

describe('productDefault', () => {
  // Lo que el producto le da a un sugerido = lo que tiene, sin lo agregado y con lo quitado.
  test('reconstruye lo sugerido a partir de los ajustes', () => {
    expect([...productDefault(['a', 'c'], ['c'], ['b'])].sort()).toEqual(['a', 'b']);
  });
});

describe('adjustedText', () => {
  test('dice los ajustes con palabras y en singular o plural', () => {
    expect(adjustedText(2, 1)).toBe('+2 agregados · −1 quitado');
    expect(adjustedText(1, 0)).toBe('+1 agregado');
    expect(adjustedText(0, 3)).toBe('−3 quitados');
    expect(adjustedText(0, 0)).toBe('');
  });
});

describe('groupByArea', () => {
  test('las áreas salen en el orden del producto, con su nombre en español', () => {
    const g = groupByArea([
      { code: 'roles.manage', area: 'team', label: 'Administrar roles' },
      { code: 'orders.cancel', area: 'orders', label: 'Cancelar pedidos' },
      { code: 'x.y', area: 'nueva', label: 'Algo nuevo' },
    ]);
    expect(g.map((a) => a.name)).toEqual(['Pedidos', 'Personal', 'Otros']);
  });
});
