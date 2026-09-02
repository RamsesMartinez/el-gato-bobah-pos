import { describe, expect, it, test } from 'vitest';
import { esEfectivo, metodoPorDefecto, metodosDeLaLista, primerMetodoLibre } from './metodosDePago';
import type { PaymentMethod } from '../types/pos';

// Ids deliberadamente ALTOS y desordenados: son los que recibiría una empresa que no es la primera
// tras volver payment_methods per-tenant. Con la lógica vieja (ids 1/2/3/7 quemados) todo esto
// fallaba en silencio.
const metodos: PaymentMethod[] = [
  { id: 9, name: 'Efectivo', kind: 'efectivo', affectsCashDrawer: true, autoDeclare: false, deliveryPlatformId: null },
  { id: 10, name: 'Tarjeta débito', kind: 'tarjeta', affectsCashDrawer: false, autoDeclare: true, deliveryPlatformId: null },
  { id: 12, name: 'Transferencia SPEI', kind: 'transferencia', affectsCashDrawer: false, autoDeclare: true, deliveryPlatformId: null },
];

test('el efectivo se reconoce por su naturaleza, no por su id', () => {
  expect(esEfectivo(metodos[0])).toBe(true);
  expect(esEfectivo(metodos[1])).toBe(false);
  expect(esEfectivo(undefined)).toBe(false);
});

test('el default no es efectivo: capturar lo recibido cuesta un paso de más', () => {
  expect(metodoPorDefecto(metodos)).toBe(10);
});

test('si el negocio solo cobra en efectivo, ese es el default', () => {
  expect(metodoPorDefecto([metodos[0]])).toBe(9);
});

test('sin métodos no hay default: la pantalla no puede inventarse uno', () => {
  expect(metodoPorDefecto([])).toBeNull();
});

test('el pago dividido no repite método', () => {
  expect(primerMetodoLibre(metodos, [9, 10])).toBe(12);
});

test('si ya se usaron todos, cae al primero en vez de quedarse sin método', () => {
  expect(primerMetodoLibre(metodos, [9, 10, 12])).toBe(9);
});

// El filtro por lista de precios. Es el espejo de la regla del servidor
// (domain.MetodoCorrespondeALaPlataforma): ofrecer un método que el backend va a rechazar deja al
// operador armando un cobro que falla con el cliente enfrente.
describe('metodosDeLaLista', () => {
  const metodos = [
    { id: 1, name: 'Efectivo', kind: 'efectivo', affectsCashDrawer: true, deliveryPlatformId: null },
    { id: 2, name: 'Tarjeta', kind: 'tarjeta', affectsCashDrawer: false, deliveryPlatformId: null },
    { id: 3, name: 'Uber Eats en línea', kind: 'plataforma', affectsCashDrawer: false, deliveryPlatformId: 5 },
    { id: 4, name: 'Uber Eats efectivo', kind: 'plataforma', affectsCashDrawer: true, deliveryPlatformId: 5 },
    { id: 5, name: 'Didi en línea', kind: 'plataforma', affectsCashDrawer: false, deliveryPlatformId: 8 },
  ] as unknown as PaymentMethod[];

  it('en mostrador solo los que no son de plataforma', () => {
    expect(metodosDeLaLista(metodos, null).map((m) => m.id)).toEqual([1, 2]);
  });

  // Los DOS de la plataforma: el repartidor a veces paga en efectivo, y ese es el motivo de que
  // exista el segundo.
  it('en una plataforma, los suyos y solo los suyos', () => {
    expect(metodosDeLaLista(metodos, 5).map((m) => m.id)).toEqual([3, 4]);
  });

  it('una plataforma sin métodos propios no cae a los de mostrador', () => {
    expect(metodosDeLaLista(metodos, 99)).toEqual([]);
  });
});
