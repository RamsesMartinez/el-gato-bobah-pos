import api from './axios';
import { FudoSale, ApiResponse } from '../../types/fudo';
import { SaleFilterService } from '../filters/saleFilters';
import { SaleFilters } from '../../types/filters';

class SaleService {
  /**
   * Obtiene ventas aplicando los filtros especificados
   * @example
   * // Obtener ventas pendientes y en curso
   * const filters = {
   *   saleState: ['PENDING', 'IN-COURSE']
   * };
   * const sales = await getSalesWithFilters(filters);
   */
  async getSalesWithFilters(filters: SaleFilters): Promise<ApiResponse<FudoSale[]>> {
    const apiFilters = SaleFilterService.buildApiFilters(filters);
    return api.get('/sales', { params: apiFilters });
  }

  /**
   * Obtiene ventas pendientes
   */
  async getPendingSales(): Promise<ApiResponse<FudoSale[]>> {
    const filters = SaleFilterService.getPendingSalesFilters();
    return this.getSalesWithFilters(filters);
  }

  /**
   * Obtiene ventas en curso
   */
  async getInProgressSales(): Promise<ApiResponse<FudoSale[]>> {
    const filters = SaleFilterService.getInProgressSalesFilters();
    return this.getSalesWithFilters(filters);
  }

  /**
   * Obtiene ventas listas para entregar
   */
  async getToDeliverSales(): Promise<ApiResponse<FudoSale[]>> {
    const filters = SaleFilterService.getToDeliverSalesFilters();
    return this.getSalesWithFilters(filters);
  }

  /**
   * Obtiene todas las ventas activas para mostrador
   */
  async getCounterSales(): Promise<ApiResponse<FudoSale[]>> {
    const filters = SaleFilterService.getCounterSalesFilters();
    return this.getSalesWithFilters(filters);
  }

  /**
   * Obtiene todas las ventas activas para domicilio
   */
  async getDeliverySales(): Promise<ApiResponse<FudoSale[]>> {
    const filters = SaleFilterService.getDeliverySalesFilters();
    return this.getSalesWithFilters(filters);
  }

  /**
   * Obtener una venta por ID
   */
  async getSaleById(id: string): Promise<ApiResponse<FudoSale>> {
    return api.get(`/sales/${id}`);
  }

  /**
   * Actualizar el estado de una venta
   */
  async updateSaleStatus(id: string, saleState: FudoSale['attributes']['saleState']): Promise<ApiResponse<FudoSale>> {
    return api.patch(`/sales/${id}`, {
      data: {
        type: "Sale",
        id,
        attributes: {
          saleState
        }
      }
    });
  }

  /**
   * Crear una nueva venta
   */
  async createSale(saleType: FudoSale['attributes']['saleType']): Promise<ApiResponse<FudoSale>> {
    return api.post('/sales', {
      data: {
        type: "Sale",
        attributes: {
          saleType
        }
      }
    });
  }
}

export const saleService = new SaleService(); 