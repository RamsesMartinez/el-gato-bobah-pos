export type SaleState = 
  | 'PENDING'
  | 'CANCELED'
  | 'CLOSED'
  | 'IN-COURSE'
  | 'PAYMENT-PROCESS'
  | 'DELIVERY-SENT'
  | 'READY_TO_DELIVER';

/**
 * Valores válidos para el tipo de venta en la API de Fudo
 * Solo se puede enviar un valor usando eq.
 * @example filter[saleType]=eq.EAT-IN
 */
export type FudoSaleType = 'EAT-IN' | 'TAKEAWAY' | 'DELIVERY';

export interface DateFilter {
  gte?: string; // greater than or equal
  lte?: string; // less than or equal
  eq?: string;  // equal
}

export interface SaleFilters {
  saleState?: SaleState[];
  /**
   * Tipo de venta. Solo se puede enviar un valor usando eq.
   * @example filter[saleType]=eq.EAT-IN
   */
  saleType?: FudoSaleType;
  createdAt?: DateFilter;
  openedAt?: DateFilter;
  closedAt?: DateFilter;
}

/**
 * Ejemplos de filtros válidos:
 * 
 * Filtro por estados:
 * {
 *   "filter[saleState]": "in.(PENDING,CANCELED,CLOSED,IN-COURSE,PAYMENT-PROCESS,DELIVERY-SENT,READY_TO_DELIVER)"
 * }
 * 
 * Filtro por estados y fecha:
 * {
 *   "filter[saleState]": "in.(PENDING,IN-COURSE)",
 *   "filter[createdAt]": "gte.2025-04-11T00:00:00Z"
 * }
 */ 