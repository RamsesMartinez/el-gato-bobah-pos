import type { PermissionInfo } from '../../api/admin';

// El permiso que el administrador no puede perder: el servidor lo rechaza (ROLE_ADMIN_KEEPS_ROLES)
// y la hoja ni siquiera lo ofrece para desmarcar.
export const ADMIN_FIXED = 'roles.manage';

// Las áreas en el orden del catálogo del servidor; un área que la pantalla todavía no conoce sale al
// final como «Otros» en vez de perderse.
const AREAS: Array<[string, string]> = [
  ['orders', 'Pedidos'], ['cash', 'Caja'], ['sales', 'Ventas y reportes'], ['expenses', 'Gastos'],
  ['inventory', 'Almacén'], ['menu', 'Menú'], ['platforms', 'Plataformas'], ['business', 'Negocio'],
  ['team', 'Personal'],
];

export interface AreaGroup {
  area: string;
  name: string;
  items: PermissionInfo[];
}

export function groupByArea(catalog: PermissionInfo[]): AreaGroup[] {
  const known = new Map(AREAS);
  const groups = AREAS.map(([area, name]) => ({ area, name, items: catalog.filter((p) => p.area === area) }));
  const other = catalog.filter((p) => !known.has(p.area));
  return [...groups, { area: 'other', name: 'Otros', items: other }].filter((g) => g.items.length > 0);
}

// permissionDiff: lo que cambió contra lo que la hoja mostró al abrir. Es lo único que viaja al
// servidor.
export function permissionDiff(original: string[], current: Set<string>): { add: string[]; remove: string[] } {
  const before = new Set(original);
  return {
    add: [...current].filter((c) => !before.has(c)),
    remove: original.filter((c) => !current.has(c)),
  };
}

// productDefault: lo que el producto le da a un rol sugerido, reconstruido desde sus ajustes.
export function productDefault(permissions: string[], added: string[], removed: string[]): Set<string> {
  const s = new Set(permissions.filter((p) => !added.includes(p)));
  for (const r of removed) s.add(r);
  return s;
}

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

export function adjustedText(added: number, removed: number): string {
  const parts: string[] = [];
  if (added > 0) parts.push(`+${plural(added, 'agregado', 'agregados')}`);
  if (removed > 0) parts.push(`−${plural(removed, 'quitado', 'quitados')}`);
  return parts.join(' · ');
}

export function countText(n: number, one: string, many: string): string {
  return plural(n, one, many);
}
