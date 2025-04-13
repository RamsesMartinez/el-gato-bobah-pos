// Tipos base para las respuestas de la API
export interface FudoResponse<T> {
  data: T[];
}

// Tipos para las relaciones
interface FudoRelationship {
  data: {
    type: string;
    id: string;
  } | null;
}

interface FudoProductsRelationship {
  data: Array<{
    type: string;
    id: string;
  }>;
}

// Tipos para las categorías
export interface FudoCategory {
  type: "ProductCategory";
  id: string;
  attributes: {
    enableOnlineMenu: boolean | null;
    name: string;
    preparationTime: number | null;
    position: number;
  };
  relationships: {
    kitchen: FudoRelationship;
    parentCategory: FudoRelationship;
    products: FudoProductsRelationship;
  };
}

// Tipos para los productos
export interface FudoProduct {
  type: "Product";
  id: string;
  attributes: {
    active: boolean;
    code: string | null;
    cost: number | null;
    description: string | null;
    enableOnlineMenu: boolean | null;
    enableQrMenu: boolean | null;
    favourite: boolean | null;
    imageUrl: string | null;
    name: string;
    position: number;
    preparationTime: number | null;
    price: number;
    sellAlone: boolean;
    stock: number | null;
    stockControl: boolean;
  };
  relationships: {
    kitchen: FudoRelationship;
    productCategory: FudoRelationship;
    productModifiersGroups: FudoProductsRelationship;
    productProportions: FudoProductsRelationship;
  };
}

// Tipos para las ventas
export interface FudoSale {
  type: "Sale";
  id: string;
  attributes: {
    closedAt: string | null;
    comment: string | null;
    createdAt: string;
    people: number | null;
    customerName: string | null;
    anonymousCustomer: {
      name: string;
    } | null;
    total: number;
    saleType: 'TAKEAWAY' | 'DELIVERY' | 'DINE_IN';
    saleState: 'PENDING' | 'IN-COURSE' | 'READY_TO_DELIVER' | 'DELIVERY-SENT' | 'CLOSED' | 'CANCELED';
    expectedPayments: any | null;
  };
  relationships: {
    customer: {
      data: null | {
        type: string;
        id: string;
      };
    };
    discounts: {
      data: any[];
    };
    items: {
      data: any[];
    };
    payments: {
      data: any[];
    };
    tips: {
      data: any[];
    };
    shippingCosts: {
      data: any[];
    };
    table: {
      data: null | {
        type: string;
        id: string;
      };
    };
    waiter: {
      data: null | {
        type: string;
        id: string;
      };
    };
    saleIdentifier: {
      data: null | {
        type: string;
        id: string;
      };
    };
  };
}

export interface FudoSaleItem {
  type: "SaleItem";
  id: string;
  attributes: {
    name: string;
    quantity: number;
    price: number;
    total: number;
    notes: string | null;
  };
  relationships: {
    product: {
      data: {
        type: "Product";
        id: string;
      };
    };
    modifiers: {
      data: Array<{
        type: "Modifier";
        id: string;
      }>;
    };
  };
}

export enum FudoSaleState {
  CLOSED = 'CLOSED',
  IN_COURSE = 'IN-COURSE',
}

export interface ApiResponse<T> {
  data: T;
  meta?: {
    pagination?: {
      total: number;
      count: number;
      per_page: number;
      current_page: number;
      total_pages: number;
    };
  };
} 