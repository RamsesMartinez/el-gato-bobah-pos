import { expect, test } from 'vitest';
import { pantallaDe } from './rutas-medidas';

// Interfaz e Integraciones se miden (spec 085). Un nombre que no esté en `pantallasMedibles` de
// server/internal/domain/uso.go se descarta allá; éste es el lado del front de esa misma lista.
test.each([
  ['/apariencia', 'apariencia'],
  ['/integraciones', 'integraciones'],
  // «Se acabó» (spec 084) entró al menú dentro de Inventario: su apertura también se mide.
  ['/se-acabo', 'se-acabo'],
])('%s se mide como %s', (ruta, nombre) => {
  expect(pantallaDe(ruta)).toBe(nombre);
});
