import { api } from './client';

// La liquidación de un pedido de plataforma: lo que dice su documento de pago.
//
// Los importes viajan como STRING, igual que el resto del dinero del sistema: el servidor los manda
// exactos y pasarlos por `number` en el camino es donde aparecen los centavos que no cuadran.

export interface Settlement {
  orderId: number;
  reportedGross: string;
  commissionAmount: string;
  /** null = el documento no declara tasa. NO es cero: cero afirmaría que la plataforma no cobró. */
  commissionPct: string | null;
  discountTotal: string;
  discountPlatform: string;
  /** Derivado por el servidor (total − plataforma). No es una columna. */
  discountRestaurant: string;
  withholdings: string;
  /** Puede ser negativo: una promoción que financió el restaurante deja al pedido en pérdida. */
  netAmount: string;
  payoutReference: string;
  documentRef: string;
  capturedAt: string;
  capturedBy: string;
}

export interface SettlementInput {
  reportedGross: string;
  commissionAmount: string;
  commissionPct: string | null;
  discountTotal: string;
  discountPlatform: string;
  withholdings: string;
  netAmount: string;
  payoutReference: string;
  documentRef: string;
}

/** Cada cifra del periodo viaja con SOBRE CUÁNTOS PEDIDOS habla y qué incluye y qué excluye. */
export interface CifraDePlataforma {
  amount: string;
  orders: number;
  incluye: string;
  excluye: string;
}

export interface PlatformMoney {
  range: { from: string; to: string };
  vendido: CifraDePlataforma;
  seQuedoLaPlataforma: CifraDePlataforma;
  llegoAlBanco: CifraDePlataforma;
  // Conteos, sin importe: cuánto dinero hay detrás de un pedido sin liquidar es justamente lo que
  // todavía no se sabe, y un $0.00 ahí se acaba sumando a algo.
  sinLiquidar: { orders: number };
  sinFolio: { orders: number };
}

export const settlementsApi = {
  // 404 cuando no hay liquidación, y es la respuesta correcta: "todavía no llega el documento" no
  // es lo mismo que "el documento dice cero".
  get: (orderId: number) => api.get<Settlement>(`/orders/${orderId}/settlement`),
  save: (orderId: number, body: SettlementInput) =>
    api.put<Settlement>(`/orders/${orderId}/settlement`, body),
  summary: (q: { preset?: string; from?: string; to?: string }) => {
    const p = new URLSearchParams();
    for (const [k, v] of Object.entries(q)) if (v) p.set(k, String(v));
    return api.get<PlatformMoney>(`/platform-settlements/summary?${p.toString()}`);
  },
};
