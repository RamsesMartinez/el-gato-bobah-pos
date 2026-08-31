import type { OrderLine, OrderView } from '../../types/pos';

// Lo que falta por entregar de un renglón. Cero para los cancelados: esa comida no se hizo, así
// que no puede aparecer como pendiente en el tablero ni impedir que el pedido se cierre.
export function faltante(l: OrderLine): number {
  if (l.cancelled) return 0;
  return Math.max(0, Number(l.quantity) - Number(l.delivered));
}

// Los renglones que todavía deben algo, en el orden en que se capturaron. Es lo que se pinta en el
// panel de entrega: mostrar los ya completos empujaría fuera de la pantalla lo que falta, que es
// justo lo que el operador vino a buscar.
export function renglonesPendientes(order: OrderView): OrderLine[] {
  return (order.lines ?? []).filter((l) => faltante(l) > 0);
}

export interface ResumenEntrega {
  total: number;
  entregados: number;
  completo: boolean;
}

// Avance del pedido contado en RENGLONES, no en piezas: "3 de 5 productos" es lo que el operador
// puede verificar de un vistazo contra lo que tiene en la charola. Contar piezas daría "11 de 14",
// un número que no corresponde a nada que se pueda ver.
export function resumenEntrega(order: OrderView): ResumenEntrega {
  const vivos = (order.lines ?? []).filter((l) => !l.cancelled);
  const entregados = vivos.filter((l) => faltante(l) === 0).length;
  return {
    total: vivos.length,
    entregados,
    // Un pedido sin renglones vivos no está completo: nadie recibió nada. Sin esto, uno cancelado
    // renglón a renglón se pintaría como entregado.
    completo: vivos.length > 0 && entregados === vivos.length,
  };
}
