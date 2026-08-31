import { expect, test } from 'vitest';
import { etiquetaEstado, etiquetaTipo } from './etiquetas';

test('los estados y tipos se leen en español', () => {
  expect(etiquetaEstado('entregada')).toBe('Entregada');
  expect(etiquetaTipo('para_llevar')).toBe('Para llevar');
});

// El valor crudo como fallback y no una cadena vacía: un estado nuevo del backend sin traducir se
// ve feo, pero una celda vacía esconde que la venta existe.
test('un valor desconocido se muestra crudo, no vacío', () => {
  expect(etiquetaEstado('pagando')).toBe('pagando');
  expect(etiquetaTipo('mesas')).toBe('mesas');
});
