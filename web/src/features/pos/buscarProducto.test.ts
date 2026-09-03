import { describe, expect, it } from 'vitest';
import { buscarProductos } from './buscarProducto';
import type { MenuProduct } from '../../types/pos';

const p = (id: number, name: string, sku: string | null): MenuProduct =>
  ({ id, name, sku, description: null, price: '0', cost: '0', margin: '0', categoryId: 1,
     type: 'simple', favorite: false, imageUrl: null, trackStock: false, groups: [] });

const catalogo = [
  p(1, 'Papas Fritas - Corte Recto - CH (170g)', 'PAP_CH'),
  p(2, 'Boneless J', 'BONE_J'),
  p(3, 'Alitas BBQ', null),
  p(4, 'Café americano', null),
];

describe('buscar un producto en la pantalla de venta', () => {
  it('encuentra por nombre', () => {
    expect(buscarProductos(catalogo, 'bone').map((x) => x.id)).toEqual([2]);
  });

  // Quien transcribe de una libreta en una tableta táctil escribe tres letras en el teclado de
  // pantalla; los códigos son más cortos que los nombres.
  it('encuentra por código', () => {
    expect(buscarProductos(catalogo, 'PAP_CH').map((x) => x.id)).toEqual([1]);
    expect(buscarProductos(catalogo, 'pap').map((x) => x.id)).toEqual([1]);
  });

  // Sin acentos ni mayúsculas: nadie escribe "Café" con acento en el teclado de pantalla mientras
  // hay cinco pedidos en la libreta.
  it('ignora acentos y mayúsculas', () => {
    expect(buscarProductos(catalogo, 'CAFE').map((x) => x.id)).toEqual([4]);
  });

  // Un producto sin código no puede reventar la búsqueda: la mayoría del catálogo migrado no tiene.
  it('un producto sin código no estorba', () => {
    expect(buscarProductos(catalogo, 'alitas').map((x) => x.id)).toEqual([3]);
  });

  // El nombre gana al código: quien escribe "bone" busca el boneless, no un producto cuyo código
  // casualmente lo contenga.
  it('lo que coincide por nombre sale primero', () => {
    const conRuido = [...catalogo, p(5, 'Refresco', 'BONE_REF')];
    expect(buscarProductos(conRuido, 'bone').map((x) => x.id)).toEqual([2, 5]);
  });

  it('sin búsqueda devuelve todo', () => {
    expect(buscarProductos(catalogo, '  ')).toHaveLength(4);
  });
});
