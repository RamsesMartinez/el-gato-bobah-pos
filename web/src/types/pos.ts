// Tipos del dominio POS (espejo del backend Go). camelCase. El DINERO viaja como string
// decimal exacto ("70.50") — nunca number en el cable (ver utils/format.ts money()). Los
// tipos internos del ticket (TicketLine/TicketModifier) sí usan number: son solo la
// previsualización local; el servidor recalcula el total autoritativo.

// Moneda ISO-4217. Hoy se opera en una a la vez; el default del local es MXN.
export type Currency = 'MXN' | 'USD';

export interface MenuOption {
  id: number;
  name: string;
  priceDelta: string;
  maxPerLine: number;
  favorite: boolean;
}

export interface MenuGroup {
  id: number;
  title: string;
  min: number;
  max: number;
  options: MenuOption[];
}

// Opción sugerida por el recomendador: pct = % de probabilidad (share del grupo).
export interface RankedOption {
  id: number;
  pct: number;
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
  // Código del producto. La mayoría del catálogo migrado de FUDO no tiene, por eso es nullable.
  sku: string | null;
  description: string | null;
  price: string;
  cost: string;
  margin: string;
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
  // Listas de precios que el operador puede elegir. "Propio" no viene: es reparto del propio
  // negocio, a precio base y sin método de pago propio.
  platforms: MenuPlatform[];
  // Solo las EXCEPCIONES capturadas a mano, indexadas por plataforma y luego por producto/opción.
  // Un id ausente usa base × (1 + margen). El dinero viaja como string decimal exacto.
  platformPrices: Record<number, Record<number, string>>;
  platformModPrices: Record<number, Record<number, string>>;
}

export interface MenuPlatform {
  id: number;
  name: string;
  markupPct: string;
}

export type ServiceType = 'mostrador' | 'para_llevar' | 'domicilio';

export interface PaymentMethod {
  id: number;
  name: string;
  kind: string;
  affectsCashDrawer: boolean;
  autoDeclare: boolean;
  // A qué plataforma pertenece, o null si no es de plataforma. Deja filtrar los métodos de la
  // lista activa sin compararlos por nombre.
  deliveryPlatformId: number | null;
}

// --- Ticket (estado local del cajero) ---

export interface TicketModifier {
  optionId: number;
  groupId: number;
  name: string;
  priceDelta: number;
  qty: number;
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
  // Nombre con el que se canta el pedido en cocina ("Tigre"). Vacío en los pedidos anteriores a
  // que existiera: a esos no se les inventa uno.
  folioName: string;
  status: string;
  serviceType: string;
  customerName: string | null;
  subtotal: string;
  deliveryFee: string;
  total: string;
  currency: Currency;
  paid: boolean;
  openedAt: string;
  lines?: OrderLine[];
}

// OrderLine es un renglón tal como lo manda el servidor.
//
// Los campos de entrega van en un tipo aparte de los que imprime el ticket porque el ticket de
// ejemplo de Ajustes no tiene un pedido real detrás: exigirle un id y un `delivered` obligaría a
// inventarlos, y un dato inventado en un tipo es el que después alguien lee como verdadero.
export interface OrderLine extends ReceiptLine {
  id: number;
  // Cuánto de este renglón ya se le dio al cliente. Es cantidad y no un booleano porque la comida
  // sale por tandas: de cinco alitas salen tres y dos siguen en la freidora.
  delivered: string;
  cancelled: boolean;
}

// ReceiptLine es lo único que necesita saber quien imprime.
export interface ReceiptLine {
  productName: string;
  quantity: string;
  unitPrice: string;
  lineTotal: string;
  notes?: string;
  modifiers?: Array<{ name: string; quantity: number; priceDelta: string }>;
}

// ReceiptOrder es un pedido visto por la impresora: sin los datos de entrega, que el papel no lleva.
export type ReceiptOrder = Omit<OrderView, 'lines'> & { lines?: ReceiptLine[] };

export interface BoardOrder {
  id: number;
  number: number;
  folioName: string;
  status: string;
  serviceType: string;
  customerName: string | null;
  total: string;
  currency: Currency;
  paid: boolean;
  openedAt: string;
  // Avance de la entrega en renglones vivos, para pintar "3 de 5" sin traerse las líneas.
  lines: number;
  linesDelivered: number;
}
