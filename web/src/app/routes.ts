import type { SessionUser } from '../stores/session';
import { can, type Permission } from './permissions';

// Qué permiso abre cada pantalla. Espejo de los RequirePermission del servidor
// (server/internal/httpapi/router.go); la barrera es allá, esto solo evita ofrecer una pantalla que
// terminaría en 403. Una ruta sin entrada queda abierta a toda sesión (/pos, /pedidos, /apariencia).
// Con varios permisos basta uno: Empleados también es donde se arman los roles.
export const ROUTE_PERMISSIONS: Record<string, readonly Permission[]> = {
  '/caja': ['cash.operate'],
  // Caja → Administrar junta las cajas (configurar caja) con los ajustes que antes vivían en Negocio
  // (configurar el negocio): abre con cualquiera de los dos (#290).
  '/caja/administrar': ['cash.configure', 'business.configure'],
  '/gastos': ['expenses.capture'],
  '/almacen': ['inventory.view'],
  '/ventas': ['sales.view'],
  '/reportes': ['reports.view'],
  '/catalogo': ['menu.configure'],
  '/plataformas': ['platforms.configure'],
  '/empleados': ['employees.manage', 'roles.manage'],
  '/negocio': ['business.configure'],
  '/impresion': ['business.configure'],
  '/integraciones': ['integrations.manage'],
  '/se-acabo': ['inventory.report_out'],
};

export function canOpen(path: string, user: SessionUser | null): boolean {
  const needed = ROUTE_PERMISSIONS[path];
  if (!needed) return true;
  return needed.some((p) => can(p, user));
}
