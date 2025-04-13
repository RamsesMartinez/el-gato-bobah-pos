export type OrderType = 'MESA' | 'PARA LLEVAR' | 'UBER EATS';
export type OrderStatus = 'NUEVO' | 'EN_PROCESO' | 'LISTO' | 'ENTREGADO' | 'CANCELADO';

export interface OrderItem {
  id: string;
  name: string;
  quantity: number;
  price: number;
  total: number;
  details?: string;
}

export interface Order {
  id: string;
  orderNumber: string;
  type: OrderType;
  status: OrderStatus;
  tableId?: string;
  items: OrderItem[];
  total: number;
  openedAt: string;
  deliveryTime?: string;
  isDelivery: boolean;
} 