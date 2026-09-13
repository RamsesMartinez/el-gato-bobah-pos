// DE UN TOQUE A UNA CELDA (spec 019).
//
// Pura: recibe números y devuelve un número. Sin DOM y sin React, para que se pruebe en tabla y
// para que la decisión que encierra —dónde empieza y dónde acaba cada zona— no dependa de montar
// una pantalla.
//
// **Aquí se redondea, y por eso el punto exacto no existe en ningún otro lado.** Si la coordenada
// viajara al servidor y se redondeara allá, el dato fino habría pasado por el cuerpo del request,
// por el log de acceso de un proxy y por la memoria del servidor. «Se borra después» es una
// intención; redondear en el origen es una garantía.

export const COLUMNAS = 12;
export const FILAS = 7;
export const CELDAS = COLUMNAS * FILAS;

export type Orientacion = 'horizontal' | 'vertical';

export interface ZonaTocada {
  celda: number;
  orientacion: Orientacion;
}

// celdaDelToque traduce un punto del ÁREA VISIBLE a una celda.
//
// Las coordenadas son las del vidrio (`clientX`/`clientY`), no las del documento: lo que este mapa
// responde es qué parte de la pantalla usa la mano, no sobre qué contenido cayó. En una pantalla
// que se desplaza, la misma zona es contenido distinto en momentos distintos — y saber qué control
// se toca ya lo responde la 017 con las acciones con nombre.
export function celdaDelToque(x: number, y: number, ancho: number, alto: number): ZonaTocada | null {
  if (ancho <= 0 || alto <= 0) return null;
  const orientacion: Orientacion = ancho >= alto ? 'horizontal' : 'vertical';
  const columnas = orientacion === 'horizontal' ? COLUMNAS : FILAS;
  const filas = orientacion === 'horizontal' ? FILAS : COLUMNAS;

  // `min` con el último índice: un toque exactamente en el borde derecho o inferior daría el índice
  // siguiente —el que no existe— y se descartaría. Acotarlo es más honesto que perder el toque.
  const col = Math.min(columnas - 1, Math.floor((x / ancho) * columnas));
  const fil = Math.min(filas - 1, Math.floor((y / alto) * filas));
  if (col < 0 || fil < 0) return null;

  return { celda: fil * columnas + col, orientacion };
}
