export interface SaleItem {
  id: string;
  product_id: string;
  quantity: number;
  unit_price: number;
  modifiers?: {
    id: string;
    quantity: number;
  }[];
  notes?: string;
}

export interface Sale {
  id: string;
  table_id?: string;
  customer_id?: string;
  items: SaleItem[];
  payment_method?: 'cash' | 'card' | 'transfer';
  status?: 'pending' | 'completed' | 'cancelled';
  total_amount: number;
}

export interface Product {
  id: string;
  name: string;
  description?: string;
  price: number;
  image_url: string;
  category: string;
  modifiers?: ProductModifier[];
}

export interface ProductModifier {
  id: string;
  name: string;
  price: number;
  max_quantity?: number;
  min_quantity?: number;
}

export interface Customer {
  id: string;
  name: string;
  email?: string;
  phone?: string;
} 