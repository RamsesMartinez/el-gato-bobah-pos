import type { BoardLine, BoardOrder } from '../../types/pos';

// Lo que falta por entregar de un renglón. El tablero solo trae renglones vivos: los cancelados no
// viajan, porque esa comida no se hizo y pedir que se entregue sería pedir lo imposible.
export function faltante(l: BoardLine): number {
  return Math.max(0, Number(l.qty) - Number(l.delivered));
}

// Los renglones que todavía deben algo, en el orden en que se capturaron.
export function pendientes(o: BoardOrder): BoardLine[] {
  return o.lines.filter((l) => faltante(l) > 0);
}

// Cuántos productos del pedido ya salieron completos. Se cuenta en RENGLONES y no en piezas:
// "3 de 5 productos" es lo que el operador puede verificar de un vistazo contra la charola;
// "11 de 14 piezas" no corresponde a nada que se pueda ver.
export function entregados(o: BoardOrder): number {
  return o.lines.filter((l) => faltante(l) === 0).length;
}
