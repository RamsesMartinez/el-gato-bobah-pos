import { describe, expect, test } from 'vitest';
import { puedeCancelarLoQueFalta } from './cuentaEnPantalla';
import type { VistaCuenta } from './cuentaEnPantalla';

// «CANCELAR LO QUE FALTA» (dueño, 2026-10-09): solo un pedido de mostrador entregado, con pagos y
// con algo por cobrar. El mismo filtro que aplica el servidor; esto solo decide si se ofrece.
const vista = (over: Partial<VistaCuenta> & { status?: string }): VistaCuenta => ({
  tipo: 'pedido', pagado: 40, falta: 60, esPlataforma: false,
  pedido: { status: over.status ?? 'entregada' },
  ...over,
} as unknown as VistaCuenta);

describe('puedeCancelarLoQueFalta', () => {
  test.each([
    ['entregado pagado a medias', vista({}), true],
    ['sin pagos se cancela entero', vista({ pagado: 0 }), false],
    ['no debe nada', vista({ falta: 0 }), false],
    ['todavía en cocina', vista({ status: 'lista' }), false],
    ['de plataforma', vista({ esPlataforma: true }), false],
    ['cuenta en captura', vista({ tipo: 'captura' }), false],
  ])('%s', (_, v, quiere) => expect(puedeCancelarLoQueFalta(v)).toBe(quiere));
});
