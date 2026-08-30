import { esEfectivo, metodoPorDefecto, primerMetodoLibre } from './metodosDePago';
import type { PaymentMethod } from '../../types/pos';

// Ids deliberadamente ALTOS y desordenados: son los que recibiría una empresa que no es la primera
// tras volver payment_methods per-tenant. Con la lógica vieja (ids 1/2/3/7 quemados) todo esto
// fallaba en silencio.
const metodos: PaymentMethod[] = [
  { id: 9, name: 'Efectivo', kind: 'efectivo', affectsCashDrawer: true, autoDeclare: false },
  { id: 10, name: 'Tarjeta débito', kind: 'tarjeta', affectsCashDrawer: false, autoDeclare: true },
  { id: 12, name: 'Transferencia SPEI', kind: 'transferencia', affectsCashDrawer: false, autoDeclare: true },
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
