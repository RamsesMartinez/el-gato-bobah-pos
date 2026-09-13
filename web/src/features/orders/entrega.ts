import type { BoardLine, BoardOrder } from '../../types/pos';

// Lo que falta por entregar de un renglón. El tablero solo trae renglones vivos: los cancelados no
// viajan, porque esa comida no se hizo y pedir que se entregue sería pedir lo imposible.
export function faltante(l: BoardLine): number {
  return Math.max(0, Number(l.qty) - Number(l.delivered));
}

// renglonesDe es la ÚNICA forma de leer los renglones de un pedido del tablero.
//
// El tipo dice que `lines` es un arreglo y el 2026-09-13 la realidad dijo otra cosa: el servidor
// mandó `null` en un pedido cuya única línea se había cancelado —un slice nil de Go se serializa
// como `null`, no como `[]`— y estas funciones, que corren al PINTAR cada tarjeta, tumbaron el
// tablero entero con «La pantalla no se pudo mostrar». Reiniciar no servía: el dato volvía igual.
//
// El servidor ya no lo manda, y eso está fijado con su propio test allá. Esto es la otra mitad, y
// no es desconfianza: **la pantalla donde se vende no se puede caer por lo que traiga una
// respuesta**. Un pedido sin renglones se pinta vacío, que es la verdad.
export function renglonesDe(o: BoardOrder): BoardLine[] {
  return o.lines ?? [];
}

// Los renglones que todavía deben algo, en el orden en que se capturaron.
export function pendientes(o: BoardOrder): BoardLine[] {
  return renglonesDe(o).filter((l) => faltante(l) > 0);
}

// Cuántos productos del pedido ya salieron completos. Se cuenta en RENGLONES y no en piezas:
// "3 de 5 productos" es lo que el operador puede verificar de un vistazo contra la charola;
// "11 de 14 piezas" no corresponde a nada que se pueda ver.
export function entregados(o: BoardOrder): number {
  return renglonesDe(o).filter((l) => faltante(l) === 0).length;
}
