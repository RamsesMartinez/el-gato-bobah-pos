import { SaleState } from '../types/filters';

/**
 * Estados de venta válidos en la API de Fudo
 */
export const SALE_STATES: { [K in SaleState]: K } = {
  'PENDING': 'PENDING',
  'CANCELED': 'CANCELED',
  'CLOSED': 'CLOSED',
  'IN-COURSE': 'IN-COURSE',
  'PAYMENT-PROCESS': 'PAYMENT-PROCESS',
  'DELIVERY-SENT': 'DELIVERY-SENT',
  'READY_TO_DELIVER': 'READY_TO_DELIVER'
} as const;

/**
 * Agrupaciones de estados para las diferentes vistas
 */
export const SALE_STATE_GROUPS = {
  PENDING: new Set<SaleState>([SALE_STATES['PENDING']]),
  IN_PROGRESS: new Set<SaleState>([SALE_STATES['IN-COURSE']]),
  TO_DELIVER: new Set<SaleState>([
    SALE_STATES['READY_TO_DELIVER'], 
    SALE_STATES['DELIVERY-SENT'],
    SALE_STATES['PAYMENT-PROCESS']
  ])
} as const;

/**
 * Verifica si un estado pertenece a un grupo específico
 */
export const isStateInGroup = (state: SaleState, group: keyof typeof SALE_STATE_GROUPS): boolean => {
  return SALE_STATE_GROUPS[group].has(state);
}; 