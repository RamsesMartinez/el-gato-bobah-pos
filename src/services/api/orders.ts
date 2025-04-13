import { Sale } from '../../types/sales';

const API_BASE_URL = 'https://api.fu.do';

export const orderService = {
  // Obtener todas las órdenes
  getOrders: async (): Promise<Sale[]> => {
    const response = await fetch(`${API_BASE_URL}/sales`);
    if (!response.ok) {
      throw new Error('Error al obtener las órdenes');
    }
    return response.json();
  },

  // Obtener una orden por ID
  getOrderById: async (orderId: string): Promise<Sale> => {
    const response = await fetch(`${API_BASE_URL}/sales/${orderId}`);
    if (!response.ok) {
      throw new Error('Error al obtener la orden');
    }
    return response.json();
  },

  // Crear una nueva orden
  createOrder: async (orderData: Omit<Sale, 'id'>): Promise<Sale> => {
    const response = await fetch(`${API_BASE_URL}/sales`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(orderData),
    });
    if (!response.ok) {
      throw new Error('Error al crear la orden');
    }
    return response.json();
  },

  // Actualizar una orden existente
  updateOrder: async (orderId: string, orderData: Partial<Sale>): Promise<Sale> => {
    const response = await fetch(`${API_BASE_URL}/sales/${orderId}`, {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(orderData),
    });
    if (!response.ok) {
      throw new Error('Error al actualizar la orden');
    }
    return response.json();
  },

  // Actualizar el estado de una orden
  updateOrderStatus: async (orderId: string, status: Sale['status']): Promise<Sale> => {
    return orderService.updateOrder(orderId, { status });
  },
}; 