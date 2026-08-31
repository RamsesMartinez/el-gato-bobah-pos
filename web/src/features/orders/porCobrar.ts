import type { BoardOrder } from '../../types/pos';

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

export function porCobrar(ordenes: BoardOrder[]): BoardOrder[] {
  return ordenes.filter((o) => !o.paid && !noSeCobran.has(o.status));
}

export interface ResumenPorCobrar {
  cuantos: number;
  monto: number;
}

export function resumenPorCobrar(ordenes: BoardOrder[]): ResumenPorCobrar {
  const pendientes = porCobrar(ordenes);
  const monto = pendientes.reduce((s, o) => s + (Number(o.total) || 0), 0);
  return { cuantos: pendientes.length, monto: Math.round(monto * 100) / 100 };
}
