import { expect, test } from 'vitest';
import { isOwnCharge } from './cuentaEnPantalla';

// Revisión de la 089: desde que se puede cobrar una cuenta desde la columna, la hoja de cobro puede
// ser de OTRO pedido que el abierto. Solo el cobro del abierto vuelve «propios» sus cambios; si no, se
// callaba el aviso de que otra tableta cobró la cuenta abierta.
test.each([
  [null, { kind: 'order', id: 7 }, false],
  [7, { kind: 'order', id: 7 }, true],
  [8, { kind: 'order', id: 7 }, false],
  [7, { kind: 'draft', id: 'd-1' }, false],
  [7, null, false],
] as const)('cobrando %s con %o abierta → propio %s', (cobrando, abierta, esperado) => {
  expect(isOwnCharge(cobrando, abierta as never)).toBe(esperado);
});
