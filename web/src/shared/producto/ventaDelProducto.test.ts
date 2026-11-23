import { describe, expect, it } from 'vitest';
import type { AdminProduct } from '../../api/admin';
import { precioWebValido, ventaInicial, ventaParaGuardar, type VentaDelProducto } from './ventaDelProducto';

const base: VentaDelProducto = { counter: true, web: true, usarMostrador: false, precioWeb: '39', allowOversell: true, description: '' };

describe('precio web', () => {
  it.each([
    ['vacío', '', false],
    ['cero', '0', false],
    ['que redondea a cero', '0.004', false],
    ['tres decimales', '39.999', false],
    ['encima del tope de dinero', '10000001', false],
    ['negativo', '-1', false],
    ['válido', '39.50', true],
  ])('%s → %s', (_n, texto, ok) => {
    expect(precioWebValido({ ...base, precioWeb: texto })).toBe(ok);
  });

  it('con «usar el de mostrador» no hay nada que validar', () => {
    expect(precioWebValido({ ...base, usarMostrador: true, precioWeb: '' })).toBe(true);
  });

  it('viaja como texto con dos decimales', () => {
    expect(ventaParaGuardar(base, null).webPrice).toBe('39.00');
  });

  it('apagar la web con el campo vacío o en cero conserva el precio guardado', () => {
    for (const texto of ['', '0', 'abc']) {
      expect(ventaParaGuardar({ ...base, web: false, precioWeb: texto }, '39.00').webPrice).toBe('39.00');
    }
  });

  it('usar el de mostrador manda null (hereda), nunca cero', () => {
    expect(ventaParaGuardar({ ...base, usarMostrador: true }, '39.00').webPrice).toBeNull();
  });
});

describe('ventaInicial', () => {
  it('un producto de una API anterior arranca en los dos lugares, al precio de mostrador y con vender sin existencia', () => {
    expect(ventaInicial({ id: 1 } as AdminProduct)).toEqual({
      counter: true, web: true, usarMostrador: true, precioWeb: '', allowOversell: true, description: '',
    });
  });
});
