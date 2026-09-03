import type { BoardOrder } from '../../types/pos';
import { round2 } from '../../domain/cobro';

// Lo que se entregó o se está preparando y todavía nadie pagó.
//
// El sistema permite mandar a cocina sin cobrar, que es lo correcto cuando el cliente paga al
// recoger. Lo que faltaba era el aviso: un pedido puede avanzar hasta "entregada" sin haberse
// cobrado, y ahí el cliente ya se fue con la comida. La tarjeta de las columnas activas sí lo
// marcaba; la lista de entregadas no, que es justo donde deja de tener remedio.
//
// Vive fuera del componente para tener test propio: es aritmética de dinero, y el día que alguien
// agregue un estado nuevo al enum hay que decidir a mano si cuenta o no.

// Lo cancelado no se cobra. Contarlo mandaría al operador a perseguir dinero que nadie debe, y a
// desconfiar del contador la próxima vez que marque algo.
const noSeCobran = new Set(['cancelada', 'reembolsada']);

// El predicado es "DEBE ALGO", no "no está pagado", y es el mismo que usa el servidor.
//
// `paid` exige un total positivo, así que un pedido de $0 llega con `paid: false` y
// `outstanding: "0"`. Con `!paid`, el badge decía "1 por cobrar · $0" y ninguna tarjeta ofrecía
// Cobrar —el botón se pinta con `outstanding > 0`—: un renglón que no se puede atender y no se va
// solo. El servidor ya lo dice por escrito en `Open`: "el recorte usa Outstanding y no Paid, que
// exige un total positivo; un pedido de $0 no está saldado pero tampoco hay nada que cobrarle".
export function porCobrar(ordenes: BoardOrder[]): BoardOrder[] {
  return ordenes.filter((o) => Number(o.outstanding) > 0 && !noSeCobran.has(o.status));
}

export interface ResumenPorCobrar {
  cuantos: number;
  monto: number;
}

export function resumenPorCobrar(ordenes: BoardOrder[]): ResumenPorCobrar {
  const pendientes = porCobrar(ordenes);
  // Lo que falta, no el total: un pedido abonado debe menos de lo que costó, y sumar el total
  // mandaría al operador a cobrar dos veces la parte que el cliente ya dejó.
  const monto = pendientes.reduce((s, o) => s + (Number(o.outstanding) || 0), 0);
  return { cuantos: pendientes.length, monto: round2(monto) };
}
