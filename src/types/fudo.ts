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
    number: string;
    status: "nuevo" | "en_proceso" | "completado" | "cancelado";
    type: "para_llevar" | "delivery" | "mesa";
    openedAt: string;
    closedAt: string | null;
    totalAmount: number;
    totalItems: number;
    customerName: string | null;
    customerPhone: string | null;
    customerEmail: string | null;
    deliveryAddress: string | null;
    deliveryInstructions: string | null;
    paymentMethod: string | null;
    paymentStatus: "pending" | "paid" | "refunded";
    notes: string | null;
  };
  relationships: {
    items: {
      data: Array<{
        type: "SaleItem";
        id: string;
      }>;
    };
    location: {
      data: {
        type: "Location";
        id: string;
      };
    };
    user: {
      data: {
        type: "User";
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