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
    enableOnlineMenu: boolean;
    name: string;
    preparationTime: number;
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
    name: string;
    description: string | null;
    price: number;
    position: number;
  };
  relationships: {
    productCategory: FudoRelationship;
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