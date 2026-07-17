// Tipos del dominio POS (espejo del backend Go). camelCase, dinero en pesos (number).

export interface MenuOption {
  id: number;
  name: string;
  priceDelta: number;
  maxPerLine: number;
}

export interface MenuGroup {
  id: number;
  title: string;
  min: number;
  max: number;
  options: MenuOption[];
}

export interface MenuCategory {
  id: number;
  name: string;
  parentId: number | null;
  sortKey: number;
  color: string | null;
  imageUrl: string | null;
}

export interface MenuProduct {
  id: number;
  name: string;
  description: string | null;
  price: number;
  cost: number;
  margin: number;
  categoryId: number;
  type: 'simple' | 'combo';
  favorite: boolean;
  imageUrl: string | null;
  trackStock: boolean;
  groups: MenuGroup[];
}

export interface Menu {
  version: number;
  categories: MenuCategory[];
  products: MenuProduct[];
}

export type ServiceType = 'mostrador' | 'para_llevar' | 'domicilio';

export interface PaymentMethod {
  id: number;
  name: string;
  kind: string;
  affectsCashDrawer: boolean;
}

// --- Ticket (estado local del cajero) ---

export interface TicketModifier {
  optionId: number;
  groupId: number;
  name: string;
  priceDelta: number;
  qty: number;
  portion?: 'A' | 'B'; // mitad-y-mitad; ausente = aplica al producto completo
}

export interface TicketLine {
  lineId: string; // uuid local; dos bebidas con distintos toppings = 2 líneas
  productId: number;
  name: string;
  unitPrice: number;
  qty: number;
  modifiers: TicketModifier[];
  notes?: string;
}

// --- Órdenes (respuesta del backend) ---

export interface OrderView {
  id: number;
  number: number;
  status: string;
  serviceType: string;
  customerName: string | null;
  total: number;
  paid: boolean;
  openedAt: string;
  lines?: Array<{
    productName: string;
    quantity: number;
    unitPrice: number;
    lineTotal: number;
    notes?: string;
    modifiers?: Array<{ name: string; quantity: number; priceDelta: number }>;
  }>;
}

export interface BoardOrder {
  id: number;
  number: number;
  status: string;
  serviceType: string;
  customerName: string | null;
  total: number;
  paid: boolean;
  openedAt: string;
}
