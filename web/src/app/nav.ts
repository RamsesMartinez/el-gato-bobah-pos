import type { IconType } from 'react-icons';
import {
  LuShoppingCart, LuClipboardList, LuWallet, LuHandCoins, LuReceiptText,
  LuPackage, LuChartColumn, LuBookOpenText, LuUsers, LuPalette, LuStore, LuUserCog, LuPrinter, LuBike,
  LuClock, LuTrash2, LuKeyRound, LuSettings, LuWarehouse,
} from 'react-icons/lu';
import type { SessionUser } from '../stores/session';
import { canOpen } from './routes';
import { can, type Permission } from './permissions';

export type ModuleId = 'sell' | 'orders' | 'cash' | 'inventory' | 'menu' | 'staff' | 'reports' | 'settings';

export interface MenuItem {
  to: string;
  label: string;
  icon: IconType;
  // Solo los que piden algo además de `ROUTE_PERMISSIONS` (routes.ts): ahí vive el resto.
  permission?: Permission;
}

export interface MenuModule {
  id: ModuleId;
  label: string;
  icon: IconType;
  items: MenuItem[];
}

// EL MENÚ LATERAL EN DOS NIVELES (spec 085, maqueta C elegida por el dueño el 2026-11-14).
//
// Vender y Pedidos son lo de todos los días y los únicos con protagonismo: van primero y abren
// directo. El resto son módulos que juntan pantallas que ya existían; ninguna ruta, nombre ni
// permiso cambió, solo dónde se encuentran. Caja y Menú abren su propia pantalla como primer destino
// para que quien ya sabía llegar no busque de más.
//
// Menú e Inventario van juntos porque las recetas del menú descuentan del almacén.
//
// «Se acabó» (#291) va dentro de Inventario, junto a «Se echó a perder», en cuanto su ruta exista: el
// dueño decidió que no sea acceso rápido.
export const MENU_MODULES: MenuModule[] = [
  { id: 'sell', label: 'Vender', icon: LuShoppingCart, items: [{ to: '/pos', label: 'Vender', icon: LuShoppingCart }] },
  { id: 'orders', label: 'Pedidos', icon: LuClipboardList, items: [{ to: '/pedidos', label: 'Pedidos', icon: LuClipboardList }] },
  { id: 'cash', label: 'Caja', icon: LuWallet, items: [
    { to: '/caja', label: 'Caja', icon: LuWallet },
    { to: '/ventas', label: 'Ventas', icon: LuReceiptText },
    { to: '/gastos', label: 'Gastos', icon: LuHandCoins },
  ] },
  { id: 'inventory', label: 'Inventario', icon: LuPackage, items: [
    { to: '/almacen', label: 'Almacén', icon: LuWarehouse },
    // Por PERMISO y no por la ruta: lo usan cajero y cocina, que no entran a Almacén (spec 039).
    { to: '/merma', label: 'Se echó a perder', icon: LuTrash2, permission: 'inventory.write_off' },
  ] },
  { id: 'menu', label: 'Menú', icon: LuBookOpenText, items: [
    { to: '/catalogo', label: 'Menú', icon: LuBookOpenText },
    { to: '/plataformas', label: 'Plataformas', icon: LuBike },
  ] },
  { id: 'staff', label: 'Personal', icon: LuUsers, items: [
    // Solo con la asistencia encendida en la empresa (visibleModules).
    { to: '/asistencia', label: 'Asistencia', icon: LuClock },
    { to: '/empleados', label: 'Empleados', icon: LuUsers },
  ] },
  { id: 'reports', label: 'Reportes', icon: LuChartColumn, items: [{ to: '/reportes', label: 'Reportes', icon: LuChartColumn }] },
  { id: 'settings', label: 'Ajustes', icon: LuSettings, items: [
    { to: '/negocio', label: 'Negocio', icon: LuStore },
    { to: '/impresion', label: 'Impresión', icon: LuPrinter },
    { to: '/integraciones', label: 'Integraciones', icon: LuKeyRound, permission: 'integrations.manage' },
    { to: '/apariencia', label: 'Interfaz', icon: LuPalette },
    { to: '/cuenta', label: 'Mi cuenta', icon: LuUserCog },
  ] },
];

// Los que abren directo y van separados del resto por una línea.
export const PRIMARY_MODULES: readonly ModuleId[] = ['sell', 'orders'];

// visibleModules dice qué módulos y destinos ve esta sesión. Solo pregunta por permisos (nunca por
// el nombre del rol) y es espejo de la barrera del servidor: ofrecer lo que respondería 403 es una
// sorpresa a media operación. Un módulo sin destinos no aparece; con uno solo, el riel lo abre
// directo.
export function visibleModules(user: SessionUser | null, opts: { attendanceEnabled: boolean }): MenuModule[] {
  return MENU_MODULES.flatMap((m) => {
    const items = m.items.filter((i) => canOpen(i.to, user)
      && (!i.permission || can(i.permission, user))
      && (i.to !== '/asistencia' || opts.attendanceEnabled));
    return items.length > 0 ? [{ ...m, items }] : [];
  });
}

// moduleOf dice en qué módulo y destino vive una ruta, por prefijo: `/catalogo/productos` es Menú y
// `/plataformas/3/emparejar` es Plataformas. Sin módulo, nada se marca como abierto.
export function moduleOf(pathname: string): { moduleId: ModuleId; itemTo: string } | null {
  for (const m of MENU_MODULES) {
    for (const i of m.items) {
      if (pathname === i.to || pathname.startsWith(`${i.to}/`)) return { moduleId: m.id, itemTo: i.to };
    }
  }
  return null;
}
