import api from './axios';
import { FudoSale, FudoResponse } from '../../types/fudo';

// Mapear estados de Fudo a nuestros estados
function mapFudoStatus(fudoStatus: string): "nuevo" | "en_proceso" | "completado" | "cancelado" {
  const statusMap: { [key: string]: "nuevo" | "en_proceso" | "completado" | "cancelado" } = {
    'NEW': 'nuevo',
    'IN-COURSE': 'en_proceso',
    'CLOSED': 'completado',
    'CANCELLED': 'cancelado'
  };
  return statusMap[fudoStatus] || 'nuevo';
}

// Mapear tipos de Fudo a nuestros tipos
function mapFudoType(fudoType: string): "para_llevar" | "delivery" | "mesa" {
  const typeMap: { [key: string]: "para_llevar" | "delivery" | "mesa" } = {
    'TAKEAWAY': 'para_llevar',
    'DELIVERY': 'delivery',
    'DINE_IN': 'mesa'
  };
  return typeMap[fudoType] || 'para_llevar';
}

// Función adaptadora para transformar los datos de Fudo al formato FudoSale
function adaptFudoOrder(order: any): FudoSale & { raw_state: string } {
  console.log('Raw order from Fudo:', order); // Para debug

  return {
    type: "Sale",
    id: order.id,
    raw_state: order.attributes.saleState, // Cambiado de state a saleState
    attributes: {
      number: `${order.attributes.number || order.id}`,
      status: mapFudoStatus(order.attributes.saleState), // Cambiado de state a saleState
      type: mapFudoType(order.attributes.type),
      openedAt: order.attributes.createdAt,
      closedAt: order.attributes.closedAt,
      totalAmount: order.attributes.total || 0,
      totalItems: order.relationships?.items?.data?.length || 0,
      customerName: order.attributes.customerName,
      customerPhone: order.attributes.customerPhone || null,
      customerEmail: order.attributes.customerEmail || null,
      deliveryAddress: order.attributes.deliveryAddress || null,
      deliveryInstructions: order.attributes.deliveryInstructions || null,
      paymentMethod: order.attributes.paymentMethod || null,
      paymentStatus: order.attributes.paymentStatus || "pending",
      notes: order.attributes.notes || null
    },
    relationships: order.relationships || {}
  };
}

export class SaleService {
  // Obtener todas las ventas
  async getSales(): Promise<FudoResponse<FudoSale>> {
    const response = await api.get<any>('/sales');
    const adaptedOrders: FudoSale[] = response.data.data.map(adaptFudoOrder);
    return { data: adaptedOrders };
  }

  // Obtener una venta por ID
  async getSaleById(id: string): Promise<FudoResponse<FudoSale>> {
    const response = await api.get<any>(`/sales/${id}`);
    const adaptedOrder = adaptFudoOrder(response.data.data);
    return { data: [adaptedOrder] };
  }

  // Crear una nueva venta
  async createSale(saleData: Partial<FudoSale>): Promise<FudoResponse<FudoSale>> {
    const response = await api.post<any>('/sales', {
      data: {
        type: "Sale",
        attributes: {
          saleType: saleData.attributes?.type === 'para_llevar' ? 'TAKEAWAY' :
                    saleData.attributes?.type === 'delivery' ? 'DELIVERY' : 'DINE_IN',
          // Otros atributos necesarios
        }
      }
    });
    const adaptedOrder = adaptFudoOrder(response.data.data);
    return { data: [adaptedOrder] };
  }

  // Actualizar una venta existente
  async updateSale(id: string, saleData: Partial<FudoSale>): Promise<FudoResponse<FudoSale>> {
    const response = await api.patch<any>(`/sales/${id}`, {
      data: {
        type: "Sale",
        id,
        attributes: {
          // Mapear los atributos según sea necesario
          saleState: saleData.attributes?.status === 'completado' ? 'CLOSED' : 'IN-COURSE',
          // Otros atributos que se necesiten actualizar
        }
      }
    });
    const adaptedOrder = adaptFudoOrder(response.data.data);
    return { data: [adaptedOrder] };
  }

  // Actualizar el estado de una venta
  async updateSaleStatus(id: string, status: FudoSale['attributes']['status']): Promise<FudoResponse<FudoSale>> {
    const response = await api.patch<any>(`/sales/${id}`, {
      data: {
        type: "Sale",
        id,
        attributes: {
          saleState: status === 'completado' ? 'CLOSED' : 'IN-COURSE'
        }
      }
    });
    const adaptedOrder = adaptFudoOrder(response.data.data);
    return { data: [adaptedOrder] };
  }
}

export const saleService = new SaleService(); 