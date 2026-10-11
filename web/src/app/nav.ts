import {
  LuShoppingCart, LuClipboardList, LuWallet, LuHandCoins, LuReceiptText,
  LuPackage, LuChartColumn, LuBookOpenText, LuUsers, LuPalette, LuStore, LuUserCog, LuPrinter, LuBike,
  LuClock,
} from 'react-icons/lu';

// NAV es la barra lateral, en orden. En la tableta de 600 px se ven los primeros 8 sin desplazar:
// lo de todos los días va arriba. «Menú» (antes «Catálogo», decisión del dueño 2026-10-07) va junto a
// Almacén porque sus recetas son lo que descuenta de ahí. «Asistencia» va DESPUÉS de «Reportes»
// para no sacar a Reportes de los 8 visibles; para el admin queda como el 9.º, abajo del pliegue.
// Quien checa casi siempre es cajero o mesero, que ven pocos ítems, y además lo tiene en la
// pantalla de bloqueo. Solo aparece con la asistencia activa (AppShell).
export const NAV = [
  { to: '/pos', icon: LuShoppingCart, label: 'Vender' },
  { to: '/pedidos', icon: LuClipboardList, label: 'Pedidos' },
  { to: '/ventas', icon: LuReceiptText, label: 'Ventas' },
  { to: '/caja', icon: LuWallet, label: 'Caja' },
  { to: '/catalogo', icon: LuBookOpenText, label: 'Menú' },
  { to: '/almacen', icon: LuPackage, label: 'Almacén' },
  { to: '/gastos', icon: LuHandCoins, label: 'Gastos' },
  { to: '/reportes', icon: LuChartColumn, label: 'Reportes' },
  { to: '/asistencia', icon: LuClock, label: 'Asistencia' },
  { to: '/plataformas', icon: LuBike, label: 'Plataformas' },
  { to: '/empleados', icon: LuUsers, label: 'Empleados' },
  { to: '/negocio', icon: LuStore, label: 'Negocio' },
  { to: '/impresion', icon: LuPrinter, label: 'Impresión' },
  { to: '/apariencia', icon: LuPalette, label: 'Interfaz' },
  { to: '/cuenta', icon: LuUserCog, label: 'Mi cuenta' },
];
