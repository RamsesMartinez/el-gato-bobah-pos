import { SaleFilters, DateFilter } from '../../types/filters';

export class SaleFilterService {
  /**
   * Convierte los filtros de la aplicación al formato esperado por la API
   * 
   * @example
   * Input:
   * {
   *   saleState: ['PENDING', 'IN-COURSE'],
   *   createdAt: { gte: '2025-04-11T00:00:00Z' }
   * }
   * 
   * Output:
   * {
   *   "filter[saleState]": "in.(PENDING,IN-COURSE)",
   *   "filter[createdAt]": "gte.2025-04-11T00:00:00Z"
   * }
   */
  static buildApiFilters(filters: SaleFilters): Record<string, string> {
    const apiFilters: Record<string, string> = {};
    
    if (filters.saleState?.length) {
      apiFilters['filter[saleState]'] = `in.(${filters.saleState.join(',')})`;
    }

    if (filters.saleType) {
      apiFilters['filter[saleType]'] = `eq.${filters.saleType}`;
    }

    if (filters.createdAt) {
      apiFilters['filter[createdAt]'] = this.buildDateFilter(filters.createdAt);
    }

    if (filters.openedAt) {
      apiFilters['filter[openedAt]'] = this.buildDateFilter(filters.openedAt);
    }

    if (filters.closedAt) {
      apiFilters['filter[closedAt]'] = this.buildDateFilter(filters.closedAt);
    }

    // Agregar ordenamiento por fecha de creación descendente
    apiFilters['sort'] = '-createdAt';

    return apiFilters;
  }

  private static buildDateFilter(dateFilter: DateFilter): string {
    if (dateFilter.eq) return `eq.${dateFilter.eq}`;
    if (dateFilter.gte) return `gte.${dateFilter.gte}`;
    if (dateFilter.lte) return `lte.${dateFilter.lte}`;
    return '';
  }

  /**
   * Construye filtros para la vista de ventas para llevar
   */
  static getTakeawaySalesFilters(): SaleFilters {
    return {
      saleState: ['PENDING', 'IN-COURSE', 'PAYMENT-PROCESS', 'READY_TO_DELIVER'],
      saleType: 'TAKEAWAY'
    };
  }

  /**
   * Construye filtros para la vista de ventas para comer en el local
   */
  static getEatInSalesFilters(): SaleFilters {
    return {
      saleState: ['PENDING', 'IN-COURSE', 'PAYMENT-PROCESS', 'READY_TO_DELIVER'],
      saleType: 'EAT-IN'
    };
  }

  /**
   * Construye filtros para la vista de domicilio
   */
  static getDeliverySalesFilters(): SaleFilters {
    return {
      saleState: ['PENDING', 'IN-COURSE', 'PAYMENT-PROCESS', 'DELIVERY-SENT', 'READY_TO_DELIVER'],
      saleType: 'DELIVERY'
    };
  }

  /**
   * Construye filtros para ventas pendientes
   */
  static getPendingSalesFilters(): SaleFilters {
    return {
      saleState: ['PENDING']
    };
  }

  /**
   * Construye filtros para ventas en curso
   */
  static getInProgressSalesFilters(): SaleFilters {
    return {
      saleState: ['IN-COURSE']
    };
  }

  /**
   * Construye filtros para ventas listas para entregar
   */
  static getToDeliverSalesFilters(): SaleFilters {
    return {
      saleState: ['READY_TO_DELIVER', 'DELIVERY-SENT', 'PAYMENT-PROCESS']
    };
  }
} 