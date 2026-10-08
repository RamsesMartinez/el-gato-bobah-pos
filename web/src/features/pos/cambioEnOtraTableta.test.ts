import { describe, expect, test } from 'vitest';
import { cambioEnOtraTableta, type FotoDeCuenta } from './cambioEnOtraTableta';

const borrador = (over: Partial<FotoDeCuenta> = {}): FotoDeCuenta => ({
  nombre: 'Siamés', estado: 'capturando', falta: 74, renglones: { a: 1, b: 2 }, ...over,
});
const pedido = (over: Partial<FotoDeCuenta> = {}): FotoDeCuenta => ({
  nombre: 'Siamés', estado: 'abierta', falta: 194, renglones: { 1: 1, 2: 1 }, ...over,
});

describe('qué se avisa cuando la cuenta abierta cambia en otra tableta (FR-017, US7)', () => {
  test.each([
    ['cobrada', pedido(), pedido({ falta: 0 }), 'Siamés se cobró en otra tableta'],
    ['cobrada en parte', pedido(), pedido({ falta: 100 }), 'Siamés se cobró en otra tableta'],
    ['cancelada', pedido(), pedido({ estado: 'cancelada' }), 'Siamés se canceló en otra tableta'],
    ['enviada', borrador(), borrador({ estado: 'enviada' }), 'Siamés se mandó a cocina en otra tableta'],
    ['descartada', borrador(), borrador({ estado: 'descartada' }), 'Siamés se descartó en otra tableta'],
    ['un renglón quitado', borrador(), borrador({ renglones: { a: 1 } }), 'Siamés cambió en otra tableta'],
    ['una cantidad que bajó', borrador(), borrador({ renglones: { a: 1, b: 1 } }), 'Siamés cambió en otra tableta'],
  ])('%s → aviso con el nombre', (_n, antes, despues, aviso) => {
    expect(cambioEnOtraTableta(antes, despues, false)).toBe(aviso);
  });

  // US7 AS2: dos tabletas agregando a la misma cuenta se suman. Avisar eso sería ruido en cada
  // toque de la otra tableta, y quien opera aprendería a ignorar el aviso que sí importa.
  test('un agregado ajeno suma sin aviso', () => {
    expect(cambioEnOtraTableta(borrador(), borrador({ renglones: { a: 2, b: 2, c: 1 } }), false)).toBeNull();
  });

  test('lo que hizo esta misma tableta no avisa', () => {
    expect(cambioEnOtraTableta(pedido(), pedido({ falta: 0 }), true)).toBeNull();
    expect(cambioEnOtraTableta(borrador(), borrador({ estado: 'enviada' }), true)).toBeNull();
  });

  test('sin foto anterior no hay con qué comparar', () => {
    expect(cambioEnOtraTableta(undefined, pedido(), false)).toBeNull();
  });

  test('sin cambios no avisa', () => {
    expect(cambioEnOtraTableta(pedido(), pedido(), false)).toBeNull();
  });

  test('una cuenta sin nombre se nombra igual', () => {
    expect(cambioEnOtraTableta(pedido({ nombre: '' }), pedido({ nombre: '', falta: 0 }), false))
      .toBe('La cuenta se cobró en otra tableta');
  });
});
