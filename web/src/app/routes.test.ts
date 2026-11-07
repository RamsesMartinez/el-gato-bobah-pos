import { describe, expect, test } from 'vitest';

import type { SessionUser } from '../stores/session';
import { canOpen, ROUTE_PERMISSIONS } from './routes';

const sesion = (permissions: string[] | undefined, role = 'admin'): SessionUser =>
  ({ id: 1, companyId: 2, name: 'Ana', role, permissions });

// El riel y la entrada por URL preguntan por PERMISO, nunca por el nombre del rol: cuando cada
// empresa arme sus roles, «gerente» puede no existir. Es UX; la barrera es RequirePermission.
describe('canOpen', () => {
  test('una ruta sin entrada queda abierta a toda sesión, aun sin permisos', () => {
    for (const p of ['/pos', '/pedidos', '/apariencia']) {
      expect(canOpen(p, sesion([]))).toBe(true);
    }
  });

  test('cada ruta cerrada pide su permiso', () => {
    const esperado: Record<string, string> = {
      '/caja': 'cash.operate', '/gastos': 'expenses.capture', '/almacen': 'inventory.view',
      '/ventas': 'sales.view', '/reportes': 'reports.view', '/catalogo': 'menu.configure',
      '/plataformas': 'platforms.configure', '/negocio': 'business.configure', '/impresion': 'business.configure',
    };
    for (const [ruta, permiso] of Object.entries(esperado)) {
      expect(canOpen(ruta, sesion([permiso])), `${ruta} con ${permiso}`).toBe(true);
      expect(canOpen(ruta, sesion(['orders.cancel'])), `${ruta} sin ${permiso}`).toBe(false);
    }
  });

  // Empleados es también la pantalla de roles: quien solo arma roles tiene que poder entrar.
  test('Empleados abre con employees.manage o con roles.manage', () => {
    expect(canOpen('/empleados', sesion(['employees.manage']))).toBe(true);
    expect(canOpen('/empleados', sesion(['roles.manage']))).toBe(true);
    expect(canOpen('/empleados', sesion(['menu.configure']))).toBe(false);
  });

  // Un admin cuyo rol perdió el permiso no entra: lo que decide es la lista, no el nombre.
  test('el nombre del rol no abre nada', () => {
    expect(canOpen('/caja', sesion([], 'admin'))).toBe(false);
    expect(canOpen('/caja', sesion(undefined, 'admin'))).toBe(false);
    expect(canOpen('/caja', sesion(['cash.operate'], ''))).toBe(true);
  });

  test('sin sesión no abre rutas cerradas', () => {
    expect(canOpen('/caja', null)).toBe(false);
    expect(canOpen('/pos', null)).toBe(true);
  });

  test('toda ruta cerrada nombra al menos un permiso', () => {
    for (const perms of Object.values(ROUTE_PERMISSIONS)) expect(perms.length).toBeGreaterThan(0);
  });
});
