import { expect, test } from 'vitest';
import { canAccess } from './roles';

// A01/UX: el gate del cliente refleja los RequireRole del backend para no mostrar opciones
// que terminarían en 403. No es seguridad (el backend sigue aplicando 403), solo UX.
test('canAccess refleja los gates del backend por rol', () => {
  // rutas libres: cualquier rol autenticado
  for (const role of ['admin', 'gerente', 'cajero', 'mesero']) {
    expect(canAccess(role, '/pos')).toBe(true);
    expect(canAccess(role, '/pedidos')).toBe(true);
  }

  // mesero: solo vender/pedidos
  expect(canAccess('mesero', '/caja')).toBe(false);
  expect(canAccess('mesero', '/gastos')).toBe(false);
  expect(canAccess('mesero', '/almacen')).toBe(false);

  // cajero: además caja, pero no gastos/almacén/reportes/catálogo/empleados
  expect(canAccess('cajero', '/caja')).toBe(true);
  expect(canAccess('cajero', '/gastos')).toBe(false);
  expect(canAccess('cajero', '/almacen')).toBe(false);
  expect(canAccess('cajero', '/empleados')).toBe(false);

  // gerente: todo menos empleados (solo admin)
  for (const p of ['/caja', '/gastos', '/almacen', '/reportes', '/catalogo']) {
    expect(canAccess('gerente', p)).toBe(true);
  }
  expect(canAccess('gerente', '/empleados')).toBe(false);

  // admin: todo
  for (const p of ['/caja', '/gastos', '/almacen', '/reportes', '/catalogo', '/empleados']) {
    expect(canAccess('admin', p)).toBe(true);
  }

  // sin rol (aún cargando / anónimo) → sin acceso a rutas restringidas
  expect(canAccess(undefined, '/caja')).toBe(false);
});
