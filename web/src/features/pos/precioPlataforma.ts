import type { Menu } from '../../types/pos';
import { round2 } from '../../domain/cobro';

// Espejo en el cliente de la regla del servidor (domain.PlatformPrice). El servidor sigue siendo la
// autoridad —recalcula todo al cobrar— pero la pantalla tiene que mostrar el mismo número, o el
// operador entrega un ticket con un total que no es el cobrado.
//
// Vive aquí y no dentro de un componente para tener test propio con los mismos casos de redondeo
// que el backend: el desacuerdo entre los dos es el riesgo principal de esta feature.

export const MOSTRADOR = null;
export type ListaActiva = number | null;

// El redondeo es el de `domain/cobro`. Aquí vivía una copia idéntica con otro nombre — misma
// decisión escrita dos veces, y la siguiente se habría escrito sin el EPSILON, como pasó en las
// otras dos copias que este mismo barrido encontró.

// precioDeLista devuelve el precio de un producto en la lista activa: el capturado a mano si
// existe, o el base más el margen. Sin plataforma (mostrador) devuelve el base sin tocar.
export function precioDeLista(menu: Menu | undefined, lista: ListaActiva, productId: number, base: number): number {
  if (lista === null || !menu) return base;
  const manual = menu.platformPrices?.[lista]?.[productId];
  if (manual !== undefined) return round2(Number(manual));
  const margen = Number(menu.platforms?.find((p) => p.id === lista)?.markupPct ?? 0);
  if (margen === 0) return base;
  return round2(base * (100 + margen) / 100);
}

// deltaDeLista: lo mismo para el cargo de una opción de modificador. Un extra sin costo sigue sin
// costo por más margen que tenga la plataforma.
export function deltaDeLista(menu: Menu | undefined, lista: ListaActiva, optionId: number, base: number): number {
  if (lista === null || !menu) return base;
  const manual = menu.platformModPrices?.[lista]?.[optionId];
  if (manual !== undefined) return round2(Number(manual));
  const margen = Number(menu.platforms?.find((p) => p.id === lista)?.markupPct ?? 0);
  if (margen === 0) return base;
  return round2(base * (100 + margen) / 100);
}

// nombreDeLista: qué se muestra en el indicador. Nunca debe quedar vacío — el operador tiene que
// saber siempre con qué lista está cobrando.
export function nombreDeLista(menu: Menu | undefined, lista: ListaActiva): string {
  if (lista === null) return 'Mostrador';
  return menu?.platforms?.find((p) => p.id === lista)?.name ?? 'Mostrador';
}

// Qué se muestra en el diálogo de captura. `null` en mostrador: la lista base se edita en el
// catálogo, no aquí, y confundir las dos es justo el error que esta feature no puede permitirse.
export interface DesglosePrecio {
  base: number;      // el de mostrador, que este diálogo nunca toca
  calculado: number; // base + margen de la plataforma
  vigente: number;   // lo que se cobra hoy: el manual si existe, si no el calculado
  esManual: boolean;
  // Cuándo lo copió la plataforma conectada. Presente = lo pone la plataforma y no se edita aquí.
  sincronizadoEn?: string;
}

// desglosePrecio abre el número en sus partes para que el operador vea de dónde sale antes de
// corregirlo. `esManual` sale de que EXISTA la fila, no de comparar contra el calculado: un precio
// capturado que coincide con el calculado sigue siendo una excepción guardada, y si no se
// distinguiera, el botón de quitarla desaparecería y quedaría atrapada en la base.
export function desglosePrecio(
  menu: Menu | undefined,
  lista: ListaActiva,
  productId: number,
  base: number,
): DesglosePrecio | null {
  if (lista === null || !menu) return null;
  const margen = Number(menu.platforms?.find((p) => p.id === lista)?.markupPct ?? 0);
  const calculado = margen === 0 ? base : round2(base * (100 + margen) / 100);
  const manual = menu.platformPrices?.[lista]?.[productId];
  if (manual === undefined) {
    return { base, calculado, vigente: calculado, esManual: false };
  }
  const sincronizadoEn = menu.platformSynced?.[lista]?.[productId];
  return { base, calculado, vigente: round2(Number(manual)), esManual: true, ...(sincronizadoEn ? { sincronizadoEn } : {}) };
}

// desgloseDelta: lo mismo para el cargo de un extra. Va aparte de desglosePrecio y no como un
// parámetro porque las dos cosas se validan distinto —un extra SÍ puede costar 0 ("sin cebolla") y
// un producto en 0 siempre es un error de captura— y juntarlas obligaría a que quien llama recuerde
// pasar el flag correcto.
export function desgloseDelta(
  menu: Menu | undefined,
  lista: ListaActiva,
  optionId: number,
  base: number,
): DesglosePrecio | null {
  if (lista === null || !menu) return null;
  const margen = Number(menu.platforms?.find((p) => p.id === lista)?.markupPct ?? 0);
  const calculado = margen === 0 ? base : round2(base * (100 + margen) / 100);
  const manual = menu.platformModPrices?.[lista]?.[optionId];
  if (manual === undefined) {
    return { base, calculado, vigente: calculado, esManual: false };
  }
  const sincronizadoEn = menu.platformModSynced?.[lista]?.[optionId];
  return { base, calculado, vigente: round2(Number(manual)), esManual: true, ...(sincronizadoEn ? { sincronizadoEn } : {}) };
}

