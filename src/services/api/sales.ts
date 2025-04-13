import { AxiosResponse } from 'axios';
import api from './axios';
import { FudoSale, FudoResponse } from '../../types/fudo';
import mockOrders from '../../mocks/fudo.orders.json';

// Función adaptadora para transformar los datos de Fudo al formato FudoSale
function adaptFudoOrder(order: any): FudoSale {
  return {
    type: "Sale",
    id: order.id,
    attributes: {
      number: order.attributes.createdAt.split('T')[0] + '-' + order.id,
      status: mapFudoStatus(order.attributes.saleState),
      type: mapFudoType(order.attributes.saleType),
      openedAt: order.attributes.createdAt,
      closedAt: order.attributes.closedAt,
      totalAmount: order.attributes.total,
      totalItems: 0, // Se calculará después
      customerName: order.attributes.customerName,
      customerPhone: null,
      customerEmail: null,
      deliveryAddress: null,
      deliveryInstructions: null,
      paymentMethod: null,
      paymentStatus: "pending",
      notes: order.attributes.comment
    },
    relationships: order.relationships
  };
}

// Mapear estados de Fudo a nuestros estados
function mapFudoStatus(fudoStatus: string): "nuevo" | "en_proceso" | "completado" | "cancelado" {
  const statusMap: { [key: string]: "nuevo" | "en_proceso" | "completado" | "cancelado" } = {
    'pending': 'nuevo',
    'in_progress': 'en_proceso',
    'completed': 'completado',
    'cancelled': 'cancelado'
  };
  return statusMap[fudoStatus] || 'nuevo';
}

// Mapear tipos de Fudo a nuestros tipos
function mapFudoType(fudoType: string): "para_llevar" | "delivery" | "mesa" {
  const typeMap: { [key: string]: "para_llevar" | "delivery" | "mesa" } = {
    'takeaway': 'para_llevar',
    'delivery': 'delivery',
    'dine_in': 'mesa'
  };
  return typeMap[fudoType] || 'para_llevar';
}

export class SaleService {
  private mockDelay = 300; // Simular delay de red

  // Obtener todas las ventas
  async getSales(): Promise<FudoResponse<FudoSale>> {
    if (process.env.NODE_ENV === 'development') {
      // Simular delay de red
      await new Promise(resolve => setTimeout(resolve, this.mockDelay));
      const adaptedOrders: FudoSale[] = mockOrders.data.map(adaptFudoOrder);
      return { data: adaptedOrders };
    }

    const response = await api.get<FudoResponse<FudoSale>>('/sales');
    return response.data;
  }

  // Obtener una venta por ID
  async getSaleById(id: string): Promise<FudoResponse<FudoSale>> {
    if (process.env.NODE_ENV === 'development') {
      await new Promise(resolve => setTimeout(resolve, this.mockDelay));
      const order = mockOrders.data.find(sale => sale.id === id);
      if (!order) return { data: [] };
      return { data: [adaptFudoOrder(order)] };
    }

    const response = await api.get<FudoResponse<FudoSale>>(`/sales/${id}`);
    return response.data;
  }

  // Crear una nueva venta
  async createSale(saleData: Partial<FudoSale>): Promise<FudoResponse<FudoSale>> {
    if (process.env.NODE_ENV === 'development') {
      await new Promise(resolve => setTimeout(resolve, this.mockDelay));
      const newSale: FudoSale = {
        type: "Sale",
        id: String(mockOrders.data.length + 1),
        attributes: {
          number: String(mockOrders.data.length + 1),
          status: "nuevo",
          type: "para_llevar",
          openedAt: new Date().toISOString(),
          closedAt: null,
          totalAmount: 0,
          totalItems: 0,
          customerName: null,
          customerPhone: null,
          customerEmail: null,
          deliveryAddress: null,
          deliveryInstructions: null,
          paymentMethod: null,
          paymentStatus: "pending",
          notes: null,
          ...saleData.attributes
        },
        relationships: {
          items: { data: [] },
          location: { data: { type: "Location", id: "1" } },
          user: { data: { type: "User", id: "1" } }
        }
      };
      return { data: [newSale] };
    }

    const response = await api.post<FudoResponse<FudoSale>>('/sales', saleData);
    return response.data;
  }

  // Actualizar una venta existente
  async updateSale(id: string, saleData: Partial<FudoSale>): Promise<FudoResponse<FudoSale>> {
    if (process.env.NODE_ENV === 'development') {
      await new Promise(resolve => setTimeout(resolve, this.mockDelay));
      const order = mockOrders.data.find(sale => sale.id === id);
      if (!order) {
        throw new Error('Venta no encontrada');
      }
      const currentSale = adaptFudoOrder(order);
      const updatedSale: FudoSale = {
        ...currentSale,
        attributes: {
          ...currentSale.attributes,
          ...saleData.attributes
        }
      };
      return { data: [updatedSale] };
    }

    const response = await api.patch<FudoResponse<FudoSale>>(`/sales/${id}`, saleData);
    return response.data;
  }

  // Actualizar el estado de una venta
  async updateSaleStatus(id: string, status: FudoSale['attributes']['status']): Promise<FudoResponse<FudoSale>> {
    const sale = await this.getSaleById(id);
    const currentSale = sale.data[0];
    
    return this.updateSale(id, {
      attributes: {
        ...currentSale.attributes,
        status
      }
    });
  }
}

export const saleService = new SaleService(); 