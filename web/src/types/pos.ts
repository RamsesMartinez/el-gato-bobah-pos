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
  // Los renglones que ACABAN de entrar, para imprimir la comanda del agregado sin volver a
  // preguntar cuáles eran. Vacío en cualquier otra respuesta.
  agregados?: number[];
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
  // Lo que falta por cobrar, tal como lo calcula el servidor. La hoja de cobro lo necesita entre
  // pedazo y pedazo de una cuenta dividida: restarlo aquí sería una segunda implementación de la
  // misma cifra, que es lo que ya dejó a la barra del POS diciendo $2,141 y a su lista $1,928.
  outstanding: string;
  openedAt: string;
  lines?: OrderLine[];
}

// CobroHecho es lo que responde cobrar: qué quedó del pedido, no un "ok".
export interface CobroHecho {
  outstanding: string;
  paid: boolean;
  // yaEstaba: este cobro ya se había registrado y esta llamada no movió dinero. Es el reintento de
  // una llamada cuya respuesta se perdió. La pantalla lo necesita para no cantar un cobro que no
  // ocurrió y para no volver a contar su propina.
  yaEstaba: boolean;
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
  // El id del renglón: es con lo que la comanda del agregado sabe cuáles imprimir. Opcional porque
  // el ticket del cliente no lo necesita y los pedidos que ya existían no lo traen.
  id?: number;
  productName: string;
  quantity: string;
  unitPrice: string;
  lineTotal: string;
  notes?: string;
  modifiers?: Array<{ name: string; quantity: number; priceDelta: string }>;
}

// ReceiptOrder es un pedido visto por la impresora: sin los datos de entrega, que el papel no lleva,
// y sin el saldo pendiente, que es asunto de la caja y no del cliente que se lleva el ticket.
export type ReceiptOrder = Omit<OrderView, 'lines' | 'outstanding'> & { lines?: ReceiptLine[] };

export interface BoardOrder {
  // Si a este pedido todavía se le puede AGREGAR. Viene del servidor y no se deduce del estado
  // aquí: la regla quedaría implementada en dos lados y se separarían al primer cambio.
  enPreparacion: boolean;
  // Renglones vivos, para que el chip diga de un vistazo qué tan grande es el pedido.
  renglones: number;
  id: number;
  number: number;
  folioName: string;
  status: string;
  serviceType: string;
  // Con qué lista se cobró. Deja ofrecer solo los métodos con los que ese pedido se puede cobrar.
  deliveryPlatformId: number | null;
  customerName: string | null;
  total: string;
  currency: Currency;
  paid: boolean;
  // Lo que falta por cobrar. Viaja aparte de `paid` porque un pedido puede estar ABONADO, y
  // derivar el pendiente del total cobraría de más.
  outstanding: string;
  openedAt: string;
  // Los renglones vivos con lo que falta de cada uno. Vienen en la misma respuesta del tablero:
  // se pintan desplegados, y pedirlos por tarjeta serían N peticiones cada diez segundos.
  // Vacío en las entregadas, que ya no tienen nada pendiente.
  lines: BoardLine[];
}

// Un renglón visto desde el tablero. Sin precio: entregar no mueve dinero, y en 600 px de alto una
// columna que no se usa le quita renglones a la que sí.
export interface BoardLine {
  id: number;
  name: string;
  qty: string;
  delivered: string;
  notes?: string;
  // "Alitas" y "Alitas BBQ sin cebolla" son platillos distintos en una cocina.
  modifiers?: string[];
}

// --- Contratos que `domain` necesita, y por eso viven aquí y no en la capa que los usa ---
//
// `domain` es puro: no importa de `api/`, de `stores/` ni de React. Estos dos tipos vivían en esos
// dos lugares, así que una función de dominio que los necesitara tenía que alcanzar hacia arriba —
// y eso es exactamente la puerta por la que se cuela el acoplamiento que este árbol quiere cerrar.

export interface TicketTab {
  id: string;
  num: number; // etiqueta estable "Cuenta N" mientras no haya nombre de cliente
  // El animal con el que se va a cantar este pedido en cocina. Se pone al ABRIR la cuenta y no al
  // cobrar, para que el operador lo vea desde el primer producto y pueda decírselo al cliente;
  // viaja al servidor con la venta y es el que acaba impreso. El servidor lo sanea y, si otro
  // pedido del día se le adelantó, le agrega la vuelta ("Tigre 2") — nunca lo cambia de animal.
  //
  // Vacío mientras la lista de animales no haya llegado del servidor. No se inventa uno: un
  // nombre que la pantalla muestra y el ticket contradice es peor que no mostrar ninguno.
  folioName: string;
  lines: TicketLine[];
  serviceType: ServiceType;
  customerName: string;
  // Con qué lista de precios se está armando esta cuenta. null = mostrador. Vive en la CUENTA y no
  // en la pantalla: se pueden tener abiertas una de mostrador y una de Uber al mismo tiempo, y
  // cada una tiene que conservar su lista.
  platformId: number | null;
}

export interface CreateOrderBody {
  clientUuid: string;
  serviceType: string;
  customerName?: string;
  notes?: string;
  // Nombre con el que la pantalla ya bautizó la cuenta. El servidor lo sanea y resuelve los
  // choques del día, así que proponerlo no es decidirlo.
  folioName?: string;
  deliveryFee?: number; // solo aplica a domicilio; el server lo ignora si no
  // Con qué lista de precios se armó. El servidor la resuelve BAJO RLS y recalcula cada precio:
  // lo que va aquí es el id, nunca los precios.
  deliveryPlatformId?: number;
  lines: Array<{
    productId: number;
    qty: number;
    notes?: string;
    modifiers: Array<{ optionId: number; qty: number }>;
  }>;
  // pago dividido: una línea por método. El pedido queda pagado cuando la suma cubre el total.
  payments?: Array<{ methodId: number; amount: number; tip?: number }>;
}
