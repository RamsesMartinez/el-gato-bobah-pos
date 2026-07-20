export type Role = 'admin' | 'gerente' | 'cajero' | 'mesero';

// Espejo de los RequireRole del backend (server/internal/httpapi/router.go). El gate del
// cliente es SOLO UX (no mostrar opciones que darían 403): el backend sigue aplicando el
// 403. Si cambian los gates del backend, actualizar aquí. Rutas sin entrada = libres para
// cualquier rol autenticado (p. ej. /pos, /pedidos, /apariencia).
export const ROUTE_ROLES: Record<string, Role[]> = {
  '/caja': ['admin', 'gerente', 'cajero'],
  '/gastos': ['admin', 'gerente'],
  '/almacen': ['admin', 'gerente'],
  '/reportes': ['admin', 'gerente'],
  '/catalogo': ['admin', 'gerente'],
  '/empleados': ['admin'],
};

export function canAccess(role: string | undefined, path: string): boolean {
  const allowed = ROUTE_ROLES[path];
  if (!allowed) return true;
  return role !== undefined && allowed.includes(role as Role);
}
