import type { Menu } from '../../types/pos';

// Espejo en el cliente de la regla del servidor (domain.PlatformPrice). El servidor sigue siendo la
// autoridad —recalcula todo al cobrar— pero la pantalla tiene que mostrar el mismo número, o el
// operador entrega un ticket con un total que no es el cobrado.
//
// Vive aquí y no dentro de un componente para tener test propio con los mismos casos de redondeo
// que el backend: el desacuerdo entre los dos es el riesgo principal de esta feature.

export const MOSTRADOR = null;
export type ListaActiva = number | null;

function redondea2(n: number): number {
  return Math.round((n + Number.EPSILON) * 100) / 100;
}

// precioDeLista devuelve el precio de un producto en la lista activa: el capturado a mano si
// existe, o el base más el margen. Sin plataforma (mostrador) devuelve el base sin tocar.
export function precioDeLista(menu: Menu | undefined, lista: ListaActiva, productId: number, base: number): number {
  if (lista === null || !menu) return base;
  const manual = menu.platformPrices?.[lista]?.[productId];
  if (manual !== undefined) return redondea2(Number(manual));
  const margen = Number(menu.platforms?.find((p) => p.id === lista)?.markupPct ?? 0);
  if (margen === 0) return base;
  return redondea2(base * (100 + margen) / 100);
}

// deltaDeLista: lo mismo para el cargo de una opción de modificador. Un extra sin costo sigue sin
// costo por más margen que tenga la plataforma.
export function deltaDeLista(menu: Menu | undefined, lista: ListaActiva, optionId: number, base: number): number {
  if (lista === null || !menu) return base;
  const manual = menu.platformModPrices?.[lista]?.[optionId];
  if (manual !== undefined) return redondea2(Number(manual));
  const margen = Number(menu.platforms?.find((p) => p.id === lista)?.markupPct ?? 0);
  if (margen === 0) return base;
  return redondea2(base * (100 + margen) / 100);
}

// nombreDeLista: qué se muestra en el indicador. Nunca debe quedar vacío — el operador tiene que
// saber siempre con qué lista está cobrando.
export function nombreDeLista(menu: Menu | undefined, lista: ListaActiva): string {
  if (lista === null) return 'Mostrador';
  return menu?.platforms?.find((p) => p.id === lista)?.name ?? 'Mostrador';
}
