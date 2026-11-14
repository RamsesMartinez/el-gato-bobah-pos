import { describe, it, expect } from 'vitest';
import { MENU_MODULES, visibleModules, moduleOf } from './nav';
import type { SessionUser } from '../stores/session';
import type { Permission } from './permissions';

// Los permisos que traen hoy los roles sugeridos (0108, más integrations.manage del admin en 0111 e
// inventory.report_out de los cuatro en la 084).
// La prueba pregunta por permisos: el nombre del rol solo da nombre al caso.
const GERENTE: Permission[] = ['payments.void', 'orders.cancel', 'orders.move_lines', 'orders.cancel_pending',
  'attendance.report', 'attendance.correct_others', 'audit.view', 'payments.change_terminal', 'cash.configure',
  'inventory.health_view', 'menu.configure', 'inventory.receive', 'inventory.configure', 'inventory.count',
  'inventory.write_off', 'inventory.write_off_approve', 'inventory.view', 'cash.close_without_clock_out',
  'cash.adjust_count', 'expenses.cancel', 'orders.delivered_view', 'cash.operate', 'orders.platform_ref',
  'platform_prices.capture', 'sales.view', 'platform_settlements.capture', 'reports.view', 'expenses.capture',
  'platforms.configure', 'business.configure', 'inventory.report_out'];
const ADMIN: Permission[] = [...GERENTE, 'employees.manage', 'roles.manage', 'integrations.manage',
  'attendance.configure', 'attendance.correct_own', 'expenses.edit'];
const CAJERO: Permission[] = ['orders.move_lines', 'orders.cancel_pending', 'inventory.write_off',
  'orders.delivered_view', 'cash.operate', 'orders.platform_ref', 'platform_prices.capture', 'inventory.report_out'];
const MESERO: Permission[] = ['orders.move_lines', 'orders.cancel_pending', 'inventory.write_off', 'inventory.report_out'];

const user = (permissions: Permission[]) => ({ id: 1, name: 'X', role: '', permissions }) as unknown as SessionUser;
const ver = (permissions: Permission[], attendanceEnabled = true) =>
  Object.fromEntries(visibleModules(user(permissions), { attendanceEnabled }).map((m) => [m.label, m.items.map((i) => i.label)]));

// Lo que ve cada rol sugerido: la tabla de specs/085-menu-en-dos-niveles/spec.md.
describe('el menú en dos niveles', () => {
  it('el admin ve los ocho, con todos sus destinos', () => {
    expect(ver(ADMIN)).toEqual({
      Vender: ['Vender'], Pedidos: ['Pedidos'],
      Caja: ['Caja', 'Ventas', 'Gastos'],
      Inventario: ['Almacén', 'Se echó a perder', 'Se acabó'],
      Menú: ['Menú', 'Plataformas'],
      Personal: ['Asistencia', 'Empleados'],
      Reportes: ['Reportes'],
      Ajustes: ['Negocio', 'Impresión', 'Integraciones', 'Interfaz', 'Mi cuenta'],
    });
  });

  it('el gerente ve los ocho, sin Empleados ni Integraciones', () => {
    const g = ver(GERENTE);
    expect(Object.keys(g)).toEqual(['Vender', 'Pedidos', 'Caja', 'Inventario', 'Menú', 'Personal', 'Reportes', 'Ajustes']);
    expect(g.Personal).toEqual(['Asistencia']);
    expect(g.Ajustes).toEqual(['Negocio', 'Impresión', 'Interfaz', 'Mi cuenta']);
  });

  it('el cajero ve seis, y Caja con un solo destino', () => {
    expect(ver(CAJERO)).toEqual({
      Vender: ['Vender'], Pedidos: ['Pedidos'], Caja: ['Caja'],
      Inventario: ['Se echó a perder', 'Se acabó'], Personal: ['Asistencia'], Ajustes: ['Interfaz', 'Mi cuenta'],
    });
  });

  it('el mesero ve cinco, sin Caja', () => {
    expect(Object.keys(ver(MESERO))).toEqual(['Vender', 'Pedidos', 'Inventario', 'Personal', 'Ajustes']);
  });

  // La asistencia es activable por empresa: apagada, Personal se queda sin destinos para el cajero.
  it('sin asistencia, Personal desaparece si no queda nada en él', () => {
    expect(ver(CAJERO, false).Personal).toBeUndefined();
    expect(ver(ADMIN, false).Personal).toEqual(['Empleados']);
  });

  // Un rol propio con un permiso suelto no ve una pantalla que respondería 403.
  it('un rol propio con solo «ver ventas» ve Caja con Ventas como único destino', () => {
    expect(ver(['sales.view'] as Permission[], false)).toEqual({
      Vender: ['Vender'], Pedidos: ['Pedidos'], Caja: ['Ventas'], Ajustes: ['Interfaz', 'Mi cuenta'],
    });
  });

  // «Se acabó» (spec 084) cuelga de su propio permiso, no del de Almacén ni del de merma: un rol propio
  // que solo reporta faltantes ve Inventario con ese único destino, y el riel lo abre directo.
  it('un rol propio con solo «reportar que se acabó» ve Inventario con Se acabó', () => {
    expect(ver(['inventory.report_out'] as Permission[], false).Inventario).toEqual(['Se acabó']);
  });

  it('sin permisos, solo lo que abre cualquier sesión', () => {
    expect(Object.keys(ver([], false))).toEqual(['Vender', 'Pedidos', 'Ajustes']);
  });

  it('Vender y Pedidos van primero', () => {
    expect(MENU_MODULES.slice(0, 2).map((m) => m.label)).toEqual(['Vender', 'Pedidos']);
  });

  // Las recetas del menú descuentan del almacén: los dos módulos quedan juntos (antes: Almacén justo
  // después de Menú en la lista plana).
  it('Menú e Inventario quedan contiguos', () => {
    const ids = MENU_MODULES.map((m) => m.id);
    expect(Math.abs(ids.indexOf('menu') - ids.indexOf('inventory'))).toBe(1);
  });

  // Ningún destino nuevo: cada uno es una ruta que ya existe en App.tsx.
  it('los destinos son las rutas de siempre', () => {
    expect(MENU_MODULES.flatMap((m) => m.items.map((i) => i.to)).sort()).toEqual([
      '/almacen', '/apariencia', '/asistencia', '/caja', '/catalogo', '/cuenta', '/empleados', '/gastos',
      '/impresion', '/integraciones', '/merma', '/negocio', '/pedidos', '/plataformas', '/pos', '/reportes', '/se-acabo', '/ventas',
    ]);
  });

  it.each([
    ['/pos', 'sell', '/pos'],
    ['/pedidos', 'orders', '/pedidos'],
    ['/caja', 'cash', '/caja'],
    ['/caja/administrar', 'cash', '/caja'],
    ['/gastos', 'cash', '/gastos'],
    ['/catalogo/productos', 'menu', '/catalogo'],
    ['/plataformas/3/emparejar', 'menu', '/plataformas'],
    ['/merma', 'inventory', '/merma'],
    ['/se-acabo', 'inventory', '/se-acabo'],
    ['/apariencia', 'settings', '/apariencia'],
  ])('%s abre dentro de %s', (path, moduleId, itemTo) => {
    expect(moduleOf(path)).toEqual({ moduleId, itemTo });
  });

  it('una ruta sin módulo no marca ninguno', () => {
    expect(moduleOf('/login')).toBeNull();
    expect(moduleOf('/posicion')).toBeNull();
  });
});
