// QUÉ ES CADA ZONA DE LA REJILLA (spec 019).
//
// 84 números sin referencia no son un mapa. La rejilla se pinta sola —una captura de la pantalla de
// un cliente lleva nombres, pedidos e importes, y no puede salir del local (FR-009)—, así que sin
// esta leyenda quien mire dentro de seis meses no sabría qué es la fila 0.
//
// **VA FECHADA, Y NO ES UN DETALLE.** El día que el POS se rediseñe, esta leyenda describe un
// layout que ya no existe. Con la fecha a la vista, quien mire datos de hace tres meses sabe que la
// referencia es de entonces; sin ella, creería que sigue vigente y movería el botón equivocado.
//
// Texto y nunca una imagen. Y deliberadamente GRUESA: describe bandas, no controles — la rejilla
// tiene la resolución de un botón, no la de un control.

// FECHA_DEL_LAYOUT: cuándo se midió esto contra la pantalla real. Se actualiza con cada rediseño
// del POS, junto con las bandas de abajo.
export const FECHA_DEL_LAYOUT = '2026-09-12';

export interface ZonaDelPos {
  // Inclusivos los dos extremos, contando desde 0.
  filas: [number, number];
  columnas: [number, number];
  que: string;
}

// Medido en 1024×600, que es el presupuesto real de la tableta. El panel de la cuenta se lleva 32 %
// del ancho, así que empieza en la columna 8 de 12.
const HORIZONTAL: ZonaDelPos[] = [
  { filas: [0, 1], columnas: [0, 7], que: 'Barra de cuentas, buscador y categorías' },
  { filas: [2, 6], columnas: [0, 7], que: 'Los productos' },
  { filas: [0, 5], columnas: [8, 11], que: 'La cuenta: sus renglones' },
  { filas: [6, 6], columnas: [8, 11], que: 'El total y el botón de cobrar' },
];

// En vertical el POS apila: el catálogo ocupa casi todo y la cuenta se reduce a una barra al pie
// con el total y el botón de cobrar.
const VERTICAL: ZonaDelPos[] = [
  { filas: [0, 1], columnas: [0, 6], que: 'Barra de cuentas, buscador y categorías' },
  { filas: [2, 10], columnas: [0, 6], que: 'Los productos' },
  { filas: [11, 11], columnas: [0, 6], que: 'La barra del pie: total y cobrar' },
];

export function zonasDelPos(orientacion: 'horizontal' | 'vertical'): ZonaDelPos[] {
  return orientacion === 'vertical' ? VERTICAL : HORIZONTAL;
}

// etiquetaDeZona describe una banda en una línea, para leerla al lado de la rejilla.
export function etiquetaDeZona(z: ZonaDelPos): string {
  const filas = z.filas[0] === z.filas[1] ? `Fila ${z.filas[0]}` : `Filas ${z.filas[0]}–${z.filas[1]}`;
  const columnas =
    z.columnas[0] === z.columnas[1] ? `columna ${z.columnas[0]}` : `columnas ${z.columnas[0]}–${z.columnas[1]}`;
  return `${filas} · ${columnas}: ${z.que}`;
}
