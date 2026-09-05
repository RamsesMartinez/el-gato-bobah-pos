import { api } from './client';
import type { PaymentMethod } from '../types/pos';

// Dinero/cantidades = string decimal exacto desde el backend (ver types/pos.ts).
export interface MethodTotal {
  methodId: number;
  name: string;
  expected: string;
  declared: string;
  difference: string;
  autoDeclare: boolean;
}
export interface CashMovement {
  id: number;
  kind: 'entrada' | 'salida';
  amount: string;
  concept: string;
  createdAt: string;
  userName: string;
  transferId: number | null; // no-null si el movimiento es una pierna de un traspaso entre cajas
  expenseId: number | null;  // no-null si es la salida de un gasto (se muestra en la sección Gastos)
}
// PAGO de gasto atribuido a un corte (sección "Gastos" del resumen). Es el pago y no el gasto:
// uno liquidado con dos medios toca dos cortes y cada uno ve solo su parte.
export interface CashExpenseLine {
  id: number;        // id del pago
  expenseId: number;
  category: string;
  supplier: string | null;
  paymentMethod: string;
  amount: string;
  currency: string;
  status: string;
}
// Descomposición jerárquica del corte: ingresos por método→concepto y egresos de efectivo.
export interface CorteBucket { concept: string; amount: string }
export interface CorteMethodBreakdown { method: string; total: string; items: CorteBucket[] }
// plataformas: lo que entró por cada plataforma, sumando sus DOS métodos (en línea y efectivo).
// Viene del servidor y no se calcula aquí: es el número contra el que se concilia el depósito.
export interface CortePlatformSubtotal { platform: string; total: string }
export interface CorteBreakdown {
  ingresos: CorteMethodBreakdown[];
  ingresosTotal: string;
  egresos: CorteBucket[];
  egresosTotal: string;
  plataformas: CortePlatformSubtotal[];
}
// Caja física (registro). La primaria recibe las ventas del POS.
export interface CashRegister {
  id: number;
  name: string;
  isPrimary: boolean;
  isActive: boolean;
  openSessionId: number | null; // no-null si la caja tiene una sesión abierta
}
export interface CashSession {
  id: number;
  registerId: number;
  registerName: string;
  isPrimary: boolean;
  status: string;
  openingCash: string;
  currency: string;
  openedAt: string;
  netMovements: string;
  totals: MethodTotal[];
  movements: CashMovement[];
  expenses: CashExpenseLine[];
  breakdown: CorteBreakdown;
  // Pedidos del turno que todavía no se entregan. Vienen del mismo predicado que bloquea el
  // cierre, así que la pantalla no puede decir "todo listo" mientras el botón rebota.
  pending: PendingOrder[];
  // Cuánto cobró cada persona. Dos estaciones cobran contra el mismo cajón, así que la
  // responsabilidad se rastrea por quien cobró y no por el mueble.
  cashiers: CashierTotal[];
}

// El efectivo va aparte porque es lo único que está en el cajón: una diferencia de arqueo solo
// puede venir de esa columna.
export interface CashierTotal {
  name: string;
  cash: string;
  other: string;
  payments: number;
}

// Un pedido que sigue sin salir. Aparece aunque ya esté cobrado: cobrado y entregado son cosas
// distintas, y lo que impide cerrar es la comida que no ha salido, no el dinero.
export interface PendingOrder {
  number: number;
  name: string;
}
// Fila del histórico de cortes.
export interface CashSessionRow {
  id: number;
  registerName: string;
  status: string;
  openingCash: string;
  currency: string;
  openedAt: string;
  closedAt: string | null;
  openedByName: string;
  closedByName: string | null;
  totalDifference: string;
  notes: string | null;
}
// Detalle de un corte (totales GUARDADOS al cerrar + movimientos).
export interface CashSessionDetail {
  id: number;
  registerName: string;
  status: string;
  openingCash: string;
  currency: string;
  openedAt: string;
  closedAt: string | null;
  openedByName: string;
  closedByName: string | null;
  notes: string | null;
  totals: MethodTotal[];
  movements: CashMovement[];
  expenses: CashExpenseLine[];
  breakdown: CorteBreakdown;
  // Las ventas que este corte cobró. Viven en el corte y no como filtro de la pantalla de Ventas:
  // ahí convivirían con el filtro de fechas y bastaría un rango que no toque el corte para llegar a
  // una pantalla vacía sin explicación.
  sales: CorteSale[];
  // Cuántas hay EN TOTAL, no cuántas llegaron. Es lo que deja decir "se muestran 200 de 340" en vez
  // de recortar en silencio, que se lee como "esto es todo".
  salesCount: number;
  salesShown: number;
  // Sin canceladas, sin reembolsadas y sin propinas. La pantalla lo declara.
  salesTotal: string;
}

export interface CorteSale {
  id: number;
  dailyNumber: number;
  folioName: string | null;
  openedAt: string;
  status: string;
  serviceType: string;
  total: string;
  refund: string;
}
export type FinancialGroup = 'operacional' | 'administrativo' | 'otro';
export type ExpenseStatus = 'pendiente' | 'pagada' | 'cancelada';
// Columnas ordenables de la lista de gastos (whitelist espejo de la del handler).
export type ExpenseSort = 'date' | 'status' | 'category' | 'supplier' | 'description' | 'amount';

export interface ExpenseCategory {
  id: number;
  name: string;
  financialGroup: FinancialGroup;
  isActive: boolean;
}
export interface Supplier {
  id: number;
  name: string;
  phone: string | null;
  notes: string | null;
  isActive: boolean;
}
export interface Expense {
  id: number;
  expenseDate: string;       // YYYY-MM-DD, fecha del DOCUMENTO
  receivedAt: string | null; // null = mercancía sin recibir (no ha tocado el almacén)
  status: ExpenseStatus;
  category: string;
  financialGroup: FinancialGroup;
  supplier: string | null;
  amount: string;
  currency: string;
  description: string | null;
  docKind: string | null;
  docFolio: string | null;
  paymentMethod: string | null; // "Tarjeta + Efectivo" cuando hubo varios
  paidAt: string | null;
  createdBy: string | null;
  itemCount: number;
}

// ---- Detalle: mercancía y pagos ----

export type ItemType = 'ingrediente' | 'producto';

export interface ExpenseItem {
  id: number;
  itemType: ItemType | null; // null = línea no inventariable (bolsa, envío, IVA)
  ingredientId: number | null;
  productId: number | null;
  itemName: string | null;
  description: string;
  quantity: string;
  unitCode: string | null;
  qtyReceived: string | null; // null = sin recibir; "0" = no llegó
  unitCost: string;
  amount: string;
  packQtyInBase: string | null;
}
export interface ExpensePayment {
  id: number;
  methodId: number;
  method: string;
  amount: string;
  paidOn: string;
  inCashCount: boolean; // atribuido a un corte
  reference: string | null;
  affectsCashDrawer: boolean;
}
export interface ExpenseDetail extends Expense {
  items: ExpenseItem[];
  payments: ExpensePayment[];
  paid: string;
}

// ---- Catálogo de artículos ----

export interface Unit {
  id: number;
  code: string;
  name: string;
  kind: 'masa' | 'volumen' | 'pieza';
  toBase: string;
}
export interface Ingredient {
  id: number;
  name: string;
  isActive: boolean;
  trackStock: boolean;
  isPackaging: boolean;
  minStock: string | null;
  currentCost: string;
  baseUnitId: number;
  baseUnitCode: string;
  baseUnitKind: string;
  category: string | null;
  onHand: string;
}
// Entrada del buscador único (ingredientes + productos con control de stock).
export interface Article {
  itemType: ItemType;
  id: number;
  name: string;
  unitCode: string;
  unitKind: string;
}
// Sugerencia de mapeo. source dice cuánto confiar: "aprendido" ya se confirmó con ese
// proveedor (autollenar); los otros son parecidos y esperan confirmación.
export interface ArticleSuggestion {
  // 'personal' no es una sugerencia de artículo: es la respuesta "esto ya se decidió que es de
  // la casa" y viene sin itemId.
  source: 'aprendido' | 'otro_proveedor' | 'catalogo' | 'personal';
  itemType: ItemType;
  itemId: number;
  itemName: string;
  score: number;
  matchedVia: string;
  packQtyInBase: string | null;
  unitId: number | null;
}

// Fila del catálogo aprendido por proveedor: qué decía el papel y a qué artículo se resolvió.
export interface SupplierItem {
  id: number;
  supplierId: number;
  supplier: string;
  rawCode: string | null;
  rawName: string;
  status: 'pendiente' | 'mapeado' | 'ignorado' | 'personal';
  itemType: ItemType | null;
  itemName: string | null;
  packQtyInBase: string | null;
  lastCost: string | null;
  lastSeenAt: string;
}

// ---- Extracción de documento ----

export interface DocLine {
  rawCode: string;
  rawName: string;
  qty: string;
  unit: string;
  unitPrice: string;
  amount: string;
  unitPriceAlt: string;
  amountAlt: string;
  status: '' | 'comprado' | 'no_disponible' | 'ajustado';
  packQty: string;
  packUnit: string;
  suggestedName: string;
  note: string;
}
export interface DocCharge { label: string; amount: string; affectsTotal: boolean }
export interface DocPayment { method: string; amount: string; reference: string }
export interface PurchaseDoc {
  kind: string;
  supplier: string;
  folio: string;
  issuedOn: string;
  currency: string;
  lines: DocLine[];
  charges: DocCharge[];
  payments: DocPayment[];
  subtotal: string;
  total: string;
  extra: { key: string; value: string }[];
  warnings: string[];
}
// Semáforo de confianza: dice si el documento se explica a sí mismo. Informativo — un pedido con
// descuento a nivel documento no cuadra por línea y sigue siendo válido.
export interface DocReconciliation {
  linesSum: string;
  chargesSum: string;
  breakdownSum: string;
  total: string;
  diff: string;
  balanced: boolean;
  paymentsSum: string;
  paymentsMatchTotal: boolean;
  subtotal: string;
  hasSubtotal: boolean;
  linesMatchSubtotal: boolean;
  unreadable: string[];
}
export interface ParsedDoc {
  doc: PurchaseDoc;
  raw: unknown; // extracción cruda: se reenvía en docRaw al crear el gasto
  reconciliation: DocReconciliation;
}
export interface StockLevel {
  item_type: string;
  item_name: string;
  on_hand: string;
  min_stock: string | null;
  unit_code: string;
}
export interface StockMovement {
  id: number;
  item_type: string;
  item_name: string;
  movement_type: string;
  quantity: string;
  reason: string | null;
  created_at: string;
}

// Cuerpos de request del detalle del gasto (lo que se envía; distinto de lo que se lee).
export interface ExpenseItemBody {
  itemType?: ItemType | '';
  ingredientId?: number;
  productId?: number;
  description: string;
  quantity: string;
  unitId?: number;
  qtyReceived?: string;
  amount: string;
  packQtyInBase?: string;
  rawCode?: string;
  rawName?: string;
  // personal: venía en el ticket pero no es del local. El backend lo aprende y no lo guarda
  // como línea del gasto.
  personal?: boolean;
}
export interface ExpensePaymentBody {
  methodId: number;
  amount: string;
  paidOn?: string;
  registerId?: number;
  reference?: string;
}

export const backofficeApi = {
  // Cajas (catálogo). `cashRegisters` = activas + estado de sesión; `allCashRegisters` = gestión.
  cashRegisters: () => api.get<{ items: CashRegister[] }>('/cash-registers'),
  allCashRegisters: () => api.get<{ items: CashRegister[] }>('/cash-registers/all'),
  createCashRegister: (name: string) => api.post<CashRegister>('/cash-registers', { name }),
  updateCashRegister: (id: number, b: { name: string; isActive: boolean }) =>
    api.patch<CashRegister>(`/cash-registers/${id}`, b),

  cashCurrent: (registerId: number) => api.get<CashSession | null>(`/cash-sessions/current?registerId=${registerId}`),
  cashOpen: (registerId: number, openingCash: number) => api.post<CashSession>('/cash-sessions', { registerId, openingCash }),
  cashClose: (registerId: number, declared: Record<string, number>, notes?: string) =>
    api.post<CashSession>('/cash-sessions/close', { registerId, declared, notes }),
  cashHistory: () => api.get<{ items: CashSessionRow[] }>('/cash-sessions'),
  cashSession: (id: number) => api.get<CashSessionDetail>(`/cash-sessions/${id}`),
  // Las ventas de un corte más allá de la primera página. El detalle trae las primeras; esto existe
  // para poder llegar al resto — un arqueo cuyas ventas no se pueden recorrer no se puede auditar.
  cashSessionSales: (id: number, page: number, pageSize: number) =>
    api.get<{ items: CorteSale[]; total: number; salesTotal: string }>(
      `/cash-sessions/${id}/sales?page=${page}&pageSize=${pageSize}`),
  cashMovement: (registerId: number, kind: 'entrada' | 'salida', amount: number, concept: string) =>
    api.post<CashSession>('/cash-sessions/movements', { registerId, kind, amount, concept }),
  // Traspaso de efectivo entre dos cajas abiertas (genera salida en origen + entrada en destino).
  cashTransfer: (fromRegisterId: number, toRegisterId: number, amount: number, note?: string) =>
    api.post<{ id: number }>('/cash-sessions/transfer', { fromRegisterId, toRegisterId, amount, note }),
  // Config de negocio (admin/gerente): qué método se declara solo al cerrar caja.
  setPaymentMethodAutoDeclare: (id: number, autoDeclare: boolean) =>
    api.patch<PaymentMethod>(`/payment-methods/${id}`, { autoDeclare }),

  // Categorías de gasto
  expenseCategories: () => api.get<{ items: ExpenseCategory[] }>('/expense-categories'),
  createExpenseCategory: (b: { name: string; financialGroup: FinancialGroup }) =>
    api.post<ExpenseCategory>('/expense-categories', b),
  updateExpenseCategory: (id: number, b: { name: string; financialGroup: FinancialGroup; isActive: boolean }) =>
    api.patch<ExpenseCategory>(`/expense-categories/${id}`, b),

  // Proveedores
  suppliers: () => api.get<{ items: Supplier[] }>('/suppliers'),
  createSupplier: (b: { name: string; phone?: string; notes?: string }) =>
    api.post<Supplier>('/suppliers', b),
  updateSupplier: (id: number, b: { name: string; phone?: string; notes?: string; isActive: boolean }) =>
    api.patch<Supplier>(`/suppliers/${id}`, b),

  // Gastos (paginado; el orden lo aplica el backend para que abarque todas las páginas)
  expenses: (params?: {
    status?: ExpenseStatus; page?: number; pageSize?: number; sort?: ExpenseSort; dir?: 'asc' | 'desc';
  }) => {
    const q = new URLSearchParams();
    if (params?.status) q.set('status', params.status);
    if (params?.page != null) q.set('page', String(params.page));
    if (params?.pageSize != null) q.set('pageSize', String(params.pageSize));
    if (params?.sort) { q.set('sort', params.sort); q.set('dir', params.dir ?? 'asc'); }
    const qs = q.toString();
    return api.get<{ items: Expense[]; total: number; page: number; pageSize: number }>(`/expenses${qs ? `?${qs}` : ''}`);
  },
  createExpense: (b: {
    expenseDate?: string;
    receivedAt?: string;
    categoryId: number;
    supplierId?: number;
    amount: string;
    description?: string;
    status: 'pendiente' | 'pagada';
    items?: ExpenseItemBody[];
    payments?: ExpensePaymentBody[];
    docKind?: string;
    docFolio?: string;
    docRaw?: unknown;
  }) => api.post<{ id: number }>('/expenses', b),
  expenseDetail: (id: number) => api.get<ExpenseDetail>(`/expenses/${id}`),
  // Agrega UN pago. Si con él los pagos cubren el importe, el gasto pasa a pagado.
  payExpense: (id: number, b: ExpensePaymentBody) => api.post<void>(`/expenses/${id}/pay`, b),
  // Marca la mercancía recibida y genera los movimientos de almacén. `received` mapea
  // idLínea → cantidad que llegó (0 = no llegó).
  receiveExpense: (id: number, b: { receivedAt?: string; received?: Record<string, string> }) =>
    api.post<void>(`/expenses/${id}/receive`, b),
  cancelExpense: (id: number, reason: string) => api.post<void>(`/expenses/${id}/cancel`, { reason }),

  // Extracción del documento → borrador. NO escribe nada: el operador confirma.
  parseDoc: (file: File) => {
    const fd = new FormData();
    fd.append('file', file);
    return api.postForm<ParsedDoc>('/expenses/parse-doc', fd);
  },

  // Catálogo de artículos (insumos) y buscador del detalle del gasto.
  units: () => api.get<{ items: Unit[] }>('/units'),
  ingredients: (onlyActive = false) =>
    api.get<{ items: Ingredient[] }>(`/ingredients${onlyActive ? '?onlyActive=true' : ''}`),
  createIngredient: (b: { name: string; baseUnitId: number; minStock?: string }) =>
    api.post<Ingredient>('/ingredients', b),
  searchArticles: (q: string) =>
    api.get<{ items: Article[] }>(`/articles?q=${encodeURIComponent(q)}`),
  // Catálogo aprendido: revisar qué mapeó el sistema y deshacer un mapeo equivocado.
  supplierItems: (params?: { supplierId?: number; page?: number; pageSize?: number }) => {
    const qs = new URLSearchParams();
    if (params?.supplierId) qs.set('supplierId', String(params.supplierId));
    if (params?.page) qs.set('page', String(params.page));
    if (params?.pageSize) qs.set('pageSize', String(params.pageSize));
    const q = qs.toString();
    return api.get<{ items: SupplierItem[]; total: number }>(`/supplier-items${q ? `?${q}` : ''}`);
  },
  forgetSupplierItem: (id: number) => api.del<void>(`/supplier-items/${id}`),

  // Sugerencias de mapeo para un renglón del documento. Solo sugiere.
  suggestArticles: (b: { supplierId?: number; rawCode?: string; rawName: string }) => {
    const qs = new URLSearchParams();
    if (b.supplierId) qs.set('supplierId', String(b.supplierId));
    if (b.rawCode) qs.set('rawCode', b.rawCode);
    qs.set('rawName', b.rawName);
    return api.get<{ items: ArticleSuggestion[] }>(`/articles/suggest?${qs}`);
  },

  stockLevels: () => api.get<{ items: StockLevel[] }>('/stock/levels'),
  stockMovements: () => api.get<{ items: StockMovement[] }>('/stock/movements'),
  createMovement: (b: {
    itemType: string;
    ingredientId?: number;
    productId?: number;
    movementType: string;
    quantity: number;
    reason?: string;
  }) => api.post<void>('/stock/movements', b),

  // Los tres reportes toman EL MISMO periodo y cada uno devuelve el `range` que realmente
  // consultó. Que lo devuelvan no es redundante: es lo que deja imprimir en la pantalla de qué
  // periodo son las cifras, y el encabezado fijo que decía "últimos 30 días" seguía diciéndolo con
  // cualquier otro rango elegido.
  reportSales: (q: ReportQuery = {}) =>
    api.get<{
      range: ReportRange;
      byDay: Array<{ business_date: string; orders: number; revenue: string }>;
      byMethod: Array<{ method: string; payments: number; total: string }>;
    }>(`/reports/sales?${qsReporte(q)}`),
  reportMargins: (q: ReportQuery = {}) =>
    api.get<{
      range: ReportRange;
      items: Array<{ product_name: string; qty: string; revenue: string; cost: string; margin: string }>;
    }>(`/reports/margins?${qsReporte({ ...q, limit: 50 })}`),
  // Propinas (pass-through, para repartir): por empleado que cobró y por día.
  reportTips: (q: ReportQuery = {}) =>
    api.get<{
      range: ReportRange;
      byEmployee: Array<{ employee: string; payments: number; tips: string }>;
      byDay: Array<{ business_date: string; tips: string }>;
    }>(`/reports/tips?${qsReporte(q)}`),
};

export type ReportPreset = '30d' | 'semana' | 'mes' | 'rango';
export interface ReportRange { from: string; to: string }
export interface ReportQuery {
  preset?: ReportPreset;
  from?: string;
  to?: string;
  limit?: number;
}

// Los vacíos NO viajan, igual que en Ventas: el servidor aplica el default solo al parámetro
// ausente, y un `from=` presente y vacío es un parámetro que vino mal, no uno que no vino.
function qsReporte(q: ReportQuery): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) {
    if (v !== undefined && v !== '') p.set(k, String(v));
  }
  return p.toString();
}
