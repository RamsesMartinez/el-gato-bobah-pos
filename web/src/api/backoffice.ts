import { api } from './client';
import type { PaymentMethod, AccountItem } from '../types/pos';

// Dinero/cantidades = string decimal exacto desde el backend (ver types/pos.ts).
export interface MethodTotal {
  methodId: number;
  name: string;
  // Cuál método es el del CAJÓN, para saber cuál se cuenta por denominaciones. No se compara por
  // nombre: los métodos de plataforma en efectivo también tocan el cajón y solo `kind` los separa.
  kind: string;
  // NULL con el arqueo ciego encendido y el turno abierto: lo que la pantalla no debe mostrar no se
  // le manda. Ocultarlo en el cliente lo dejaría legible en la respuesta.
  expected: string | null;
  declared: string;
  difference: string;
  autoDeclare: boolean;
  // Si ESTE método exige una cifra capturada para poder cerrar. Lo decide el servidor: deducirlo de
  // que el esperado sea cero se rompe cuando el esperado no viaja.
  requiresEntry: boolean;
}

// El arqueo del cajón físico: una cifra esperada, un conteo, una diferencia.
//
// `null` en el turno = no hay arqueo de efectivo (un corte anterior a la spec 015, o una caja que
// no maneja efectivo). `expected` en null con el arqueo ciego; `counted` y `difference` en null
// mientras el turno esté abierto.
export interface ArqueoDelCajon {
  expected: string | null;
  counted: string | null;
  difference: string | null;
  methodIds: number[];
  requiresCount: boolean;
}
export interface CashMovement {
  id: number;
  kind: 'entrada' | 'salida' | 'propina' | 'reverso' | 'retiro';
  amount: string;
  concept: string;
  createdAt: string;
  userName: string;
  transferId: number | null; // no-null si el movimiento es una pierna de un traspaso entre cajas
  expenseId: number | null;  // no-null si es la salida de un gasto (se muestra en la sección Gastos)
  // La salida de caja de una devolución (spec 029): se nombra Devolución, como en el desglose.
  isRefund?: boolean;
  // Corrección por reverso (spec 032): `reversesId` en el reverso, `reversed` en la salida corregida.
  reversesId?: number | null;
  reversed?: boolean;
  // A quién se le dio: propina entregada o retiro al cierre (spec 035).
  recipientName?: string | null;
}

// El retiro al cierre (spec 035): lo contado menos el fondo que se dejó, y quién se lo llevó.
export interface Withdrawal {
  amount: string;
  userName: string;
}

export interface CashConcept {
  id: number;
  name: string;
  categoryId: number | null;
  categoryName: string | null;
  supplierId: number | null;
  supplierName: string | null;
}

// TerminalCount es el arqueo de una terminal en un turno. `expectedNow`/`differenceNow` (spec 036):
// lo que se espera hoy, distinto de lo firmado si alguien cambió la terminal de un pago después del
// arqueo. Opcionales: un backend anterior no los manda.
export interface TerminalCount {
  terminalId: number;
  name: string;
  expected: string;
  declared: string;
  difference: string;
  expectedNow?: string;
  differenceNow?: string;
}

export interface CardTerminal {
  id: number;
  branchId: number;
  branchName: string;
  name: string;
  archived: boolean;
}

export type CashMovementInput =
  | { kind: 'entrada'; amount: number; concept: string }
  | { kind: 'salida'; amount: number; conceptId: number };
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
// note: por qué un medio salió en negativo en el turno (spec 029). Solo viene cuando lo está.
export interface CorteMethodBreakdown { method: string; total: string; items: CorteBucket[]; note?: string }
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
// Caja para la gestión (`/cash-registers/all`): trae el fondo fijo, que la lista del cajero no lleva
// porque la apertura se cuenta a ciegas (spec 035).
export interface CashRegisterConfig {
  id: number;
  name: string;
  isPrimary: boolean;
  isActive: boolean;
  fixedFloat?: string | null;
}
// Una pieza que se puede contar. `value` es string por lo mismo que el resto del dinero: es una
// columna `numeric(10,2)` y convertirla a número en la frontera pierde exactitud.
export interface Denomination {
  id: number;
  value: string;
  isCoin: boolean; // solo sirve para agrupar en pantalla, y agrupar es lo que hace encontrar la pieza
}

// Los DOS CAMINOS de declarar el fondo, en un tipo que no deja mandar los dos (FR-014/FR-015).
//
// El servidor los rechaza si llegan juntos, pero llegar hasta el rechazo con el cajón contado es un
// conteo perdido: la unión discriminada lo vuelve imposible de escribir desde aquí.
export type AperturaInput = (
  | { counts: { denominationId: number; pieces: number }[] }
  | { openingCash: number; manualReason: string }
) & { openingReason?: string; openingReasonNote?: string }; // spec 032: si no coincide con el cierre anterior

// Un renglón del desglose. `subtotal` viene calculado del servidor aunque sea derivable: lo lee un
// humano comparando contra su cajón, y dos multiplicaciones del mismo dato pueden diferir.
export interface ConteoLine {
  value: string;
  isCoin: boolean;
  pieces: number;
  subtotal: string;
}
// El arqueo de un momento del turno. `manualReason` no nulo = no se contó por denominaciones, y por
// qué; en ese caso `lines` viene vacío.
export interface Conteo {
  total: string;
  manualReason: string | null;
  lines: ConteoLine[];
}
// Un momento en null = ese arqueo no se contó. Es el caso de todos los cortes anteriores a la
// funcionalidad, y la pantalla los muestra como siempre (FR-008).
export interface ConteosDelTurno {
  apertura: Conteo | null;
  cierre: Conteo | null;
}

// Lo que la asistencia pide para la acción que toca en una caja (spec 046): abrirla si está
// cerrada, cerrarla si está abierta. `person` es quien abre (`open`) o quien abrió (`close`).
export interface CashAttendanceGate {
  stage: 'open' | 'close';
  applies: boolean;
  ready: boolean;
  person: { id: number; name: string; active: boolean };
  openSince?: string;
  stale: boolean;
  isSelf: boolean;
  canAuthorize: boolean;
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
  // El turno se está contando a ciegas: `breakdown` y `cashiers` vienen SIN las cifras de venta.
  // Lo dice el servidor y no se deduce de que las listas vengan vacías — un turno sin ventas las
  // trae vacías también, y la pantalla diría «Sin ingresos» sobre las dos.
  blind?: boolean;
  // Pedidos del turno que todavía no se entregan. Vienen del mismo predicado que bloquea el
  // cierre, así que la pantalla no puede decir "todo listo" mientras el botón rebota.
  pending: PendingOrder[];
  // Cuánto cobró cada persona. Dos estaciones cobran contra el mismo cajón, así que la
  // responsabilidad se rastrea por quien cobró y no por el mueble.
  cashiers: CashierTotal[];
  // La venta del turno que ningún pago cubre. Es la hermana de `pending`: aquélla dice qué comida
  // no ha salido, ésta qué dinero no entró. No bloquea el cierre.
  uncollected: string;
  uncollectedCount: number;
  // Lo dado por perdido («cancelar lo que falta», 2026-10-09): ni cobrado ni sin cobrar.
  writtenOff?: string;
  counts: ConteosDelTurno | null;
  drawer: ArqueoDelCajon | null;
  // Pagos devueltos en el turno (spec 027). Lista aparte y no una salida: el esperado ya los
  // excluye. Opcional para que el compilador obligue a la guarda ante un backend viejo.
  voidedPayments?: VoidedPayment[];
  // Las cuentas vivas que NO bloquean el cierre (spec 030, D-10): las que se capturan y las
  // entregadas que deben, de cualquier día. Opcional para que el compilador obligue a la guarda.
  liveAccounts?: AccountItem[];
  // Los entregados de mostrador que deben, de cualquier día. BLOQUEAN el cierre (no hay fiados,
  // 2026-10-09) y salen del mismo predicado que la guardia del servidor. Opcional: un servidor que
  // todavía no lo manda deja la sección sin pintar, no rota.
  owing?: OwingOrder[];
  // Dinero devuelto al cliente en el turno (spec 031). Opcional por la misma razón que el anterior.
  refunds?: SessionRefund[];
  // Propinas (spec 032). Opcionales: un servidor viejo no las manda y la sección no se pinta.
  tipsPending?: TipsPending | null;
  // Arqueo de tarjeta por terminal (spec 032): lo que se pide al cerrar y, cerrado, la diferencia.
  cardCountMode?: 'auto' | 'per_terminal';
  cashOutsWithoutConcept?: number;
  terminalsToCount?: { terminalId: number; name: string }[];
  terminalCounts?: TerminalCount[];
  tipsPaidOut?: string;
  cardTipsPaidInCash?: string;
  // Fondo fijo y retiro (spec 035). `fixedFloat` solo en el turno abierto: lo que la hoja de cierre
  // pone en el campo. Opcionales: un servidor anterior no los manda.
  fixedFloat?: string | null;
  floatLeft?: string | null;
  fixedFloatAtClose?: string | null;
  withdrawal?: Withdrawal | null;
}

// Propina por entregar del turno abierto y a quién se le puede entregar.
export interface TipsPending {
  total: string;
  cash: string;
  nonCash: string;
  inherited: string;
  people?: { id: number; name: string }[];
}

export type TipsDecision = 'quedan_en_caja';

export interface TipPayoutInput {
  mode: 'parejo' | 'ajustado';
  recipients: { userId: number; amount?: number }[];
}

// Una devolución del turno. `fromDrawer`: salió del cajón (con su salida de caja); si no, ya bajó
// el esperado de su medio.
export interface SessionRefund {
  method: string;
  amount: string;
  tip: string;
  orderFolio: string;
  fromDrawer: boolean;
  refundedBy: string;
  refundedAt: string;
  reason: string;
}

export interface VoidedPayment {
  method: string;
  amount: string;
  tip: string;
  orderFolio: string;
  voidedBy: string;
  voidedAt: string;
  reason: string;
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
  // El pedido, para «Abrir» su cuenta desde el cierre. Opcional: un servidor que todavía no lo
  // manda deja el renglón sin el botón, no roto.
  id?: number;
  // De cuánto es, para no tener que abrirlo. Opcional por la misma razón.
  total?: string;
}
// Un pedido entregado que todavía debe.
export interface OwingOrder {
  id: number;
  number: number;
  name: string;
  total: string;
  paid: string;
  // Lo ya dado por perdido; con todo el resto perdido el pedido ya no aparece aquí.
  writtenOff?: string;
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
  // Null con el arqueo oculto (spec 035, R8).
  totalDifference: string | null;
  notes: string | null;
  // Con arqueo ciego, el último cierre de una caja no reabierta no muestra su arqueo a quien no
  // configura la caja. Opcional: un backend viejo no lo manda.
  arqueoHidden?: boolean;
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
  // Ver CashSession.arqueoHidden: con él, totales, arqueo, conteo de cierre e ingresos vienen vacíos.
  arqueoHidden?: boolean;
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
  // Lo dado por perdido de los pedidos del corte («cancelar lo que falta», 2026-10-09).
  writtenOff?: string;
  counts: ConteosDelTurno | null;
  drawer: ArqueoDelCajon | null;
  // Pagos devueltos en el turno (spec 027). Lista aparte y no una salida: el esperado ya los
  // excluye. Opcional para que el compilador obligue a la guarda ante un backend viejo.
  voidedPayments?: VoidedPayment[];
  refunds?: SessionRefund[];
  // Arqueo por terminal y propinas del turno (spec 032). Opcionales: un backend viejo no los manda.
  cardCountMode?: string;
  terminalCounts?: TerminalCount[];
  tipsPaidOut?: string;
  cardTipsPaidInCash?: string;
  // La propina pendiente que el cierre dejó en caja para el siguiente turno.
  tipsCarriedOver?: string;
  // Lo que se dejó de fondo, contra qué fijo, y el retiro (spec 035). Nulos en cierres de antes.
  floatLeft?: string | null;
  fixedFloatAtClose?: string | null;
  withdrawal?: Withdrawal | null;
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
  expenseDate: string;       // YYYY-MM-DD, DÍA DEL GASTO (spec 032)
  documentDate?: string | null; // YYYY-MM-DD, fecha del documento
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

// Cómo cambia el costo de un insumo al comprarlo: el de la última compra o el promedio ponderado.
export type CostingMethod = 'last' | 'weighted';

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
  // Spec 038. `applied`: lo que el renglón ya sumó al almacén, en `baseUnitCode`; se devuelve como
  // `seenApplied` al corregir. `onHand`: lo guardado hoy del artículo (null fuera del almacén).
  applied: string;
  baseUnitCode: string | null;
  notComing: string;
  isGift: boolean;
  presentationId: number | null;
  onHand?: string | null;
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
  kind: 'masa' | 'volumen' | 'pieza' | 'longitud';
  toBase: string;
}

// Un empaque de compra de un insumo (spec 038): «Caja = 12 L». `qtyInBase` va en la unidad base
// del insumo (ml); el contenido se copia a cada compra, así que editarlo no toca las pasadas.
export interface Presentation {
  id: number;
  ingredientId: number;
  name: string;
  qtyInBase: string;
  baseUnitCode: string;
  active: boolean;
  // Se compra en otro tipo de unidad que el insumo («Kilo ≈ 4 piezas»).
  approximate: boolean;
  purchaseUnitId: number | null;
}
export interface PresentationBody {
  name: string;
  qty: string;
  unitId: number;
  approximate: boolean;
  active?: boolean;
}
export interface CountPlace {
  id: number;
  name: string;
  position: number;
  ingredients: number;
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
  // Se prepara en el local con otros insumos, y si su composición está estimada o confirmada.
  isPrep?: boolean;
  compositionStatus?: '' | 'estimated' | 'confirmed';
  // En cuántas recetas aparece: los más usados van arriba en el buscador.
  recipeUses?: number;
  // Dónde está, para ordenar el conteo (spec 038).
  countPlace?: string | null;
  countPlaceId?: number | null;
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
  // Un artículo que nace con este gasto (spec 042): se crea al guardar, no al elegir su unidad.
  newArticle?: { name: string; baseUnitId: number };
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
  // El empaque del insumo: el servidor copia su contenido y fija la unidad (spec 038).
  presentationId?: number;
  // Llegó sin costo: importe 0, suma a lo guardado y no mueve el costo.
  isGift?: boolean;
}

// Un renglón que falta por llegar (spec 038). Cantidades en la unidad del renglón: el empaque
// (`presentationName`, con `packQtyInBase` en `baseUnitCode`) o la unidad (`unitCode`).
export interface PendingReceipt {
  expenseId: number;
  expenseDate: string;
  supplier: string | null;
  expenseAmount: string;
  paid: boolean;
  itemId: number;
  itemName: string;
  quantity: string;
  received: string;
  notComing: string;
  missing: string;
  applied: string;
  packQtyInBase: string | null;
  presentationName: string | null;
  unitCode: string | null;
  baseUnitCode: string | null;
}
// Conteo (spec 038). A ciegas: lo que el sistema tiene no viene mientras se cuenta.
export interface CountItem {
  ingredientId: number;
  name: string;
  place: string | null;
  placeId?: number | null;
  baseUnitCode: string;
  presentations: { name: string; qtyInBase: string }[];
  countedQty: string | null; // null = no contado
  // Un artículo en piezas que se puede contar pesando (spec 042): el servidor convierte a piezas.
  weighable?: boolean;
}
// Lo que se manda al capturar. confirmable pide el aviso de una cifra fuera de lo razonable.
export interface CountCaptureBody {
  countedQty: string;
  weight?: string;
  weightUnitId?: number;
  confirmable: true;
  confirmed?: boolean;
}
export interface StockCount {
  id: number;
  openedAt: string;
  items: CountItem[];
}
export interface CountClose {
  adjusted: { ingredientId: number; name: string; baseUnitCode: string; system: string; counted: string; delta: string }[];
  notCounted: { ingredientId: number; name: string }[];
}

// «Llegó algo más»: un renglón que no venía y ya llegó. Con `isGift`, `amount` va en 0.
export interface ArrivedItemBody {
  itemType: ItemType;
  ingredientId?: number;
  productId?: number;
  description: string;
  quantity: string;
  presentationId?: number;
  unitId?: number;
  amount: string;
  isGift: boolean;
}

// Un renglón de «Corregir lo que llegó» (spec 038). Cantidades en la unidad del renglón.
export interface StockLineBody {
  id?: number;
  seenApplied?: string;
  itemType: ItemType;
  ingredientId?: number;
  productId?: number;
  description: string;
  quantity: string;
  received: string;
  notComing: string;
  presentationId?: number;
  unitId?: number;
  amount: string;
  isGift?: boolean;
}
export interface ExpensePaymentBody {
  methodId: number;
  amount: string;
  paidOn?: string;
  registerId?: number;
  reference?: string;
}

// Salud del inventario (spec 037). Las listas se declaran opcionales a propósito: un `null` del
// servidor tumbaría la pantalla sin un solo error, y así el compilador obliga a la guarda.
export type HealthSection = 'unsold_deduction' | 'negative_stock' | 'no_purchases' | 'malformed_recipe' | 'pending_recipe';
export interface HealthUnsoldItem {
  kind: 'product' | 'extra';
  id: number;
  name: string;
  group: string;
  // own_stock | recipe | package | none | undecided en un producto; 'extra' en un extra.
  status: string;
  units?: { channel: string; units: string }[];
  pieces: string;
  linesWithoutDeduction: number;
  // null cuando nadie ha capturado el costo: no es «$0».
  estimatedCost: string | null;
  fixAt: string;
}
export interface HealthComboChoice { productId: number; name: string; slot: string; options: number; fixAt: string }
export interface HealthNegative { ingredientId: number; name: string; unit: string; onHand: string; branch: string | null; usedBy: number; fixAt: string }
export interface HealthNoPurchase { ingredientId: number; name: string; unit: string; fixAt: string }
export interface HealthMalformed {
  ownerKind: 'product' | 'extra' | 'prep';
  ownerId: number;
  ownerName: string;
  ingredient: string;
  problem: 'zero_quantity' | 'unit_mismatch' | 'archived_ingredient' | 'duplicate_ingredient';
  fixAt: string;
}
export interface HealthPendingCount { products: number; extras: number; fixAt: string }
export interface InventoryHealth {
  from: string;
  to: string;
  multipleBranches: boolean;
  salesInPeriod: number;
  unsoldDeduction: { total: number; items?: HealthUnsoldItem[]; unpairedPlatformLines: number };
  comboChoices: { total: number; items?: HealthComboChoice[] };
  negativeStock: { total: number; collapsed: boolean; items?: HealthNegative[] };
  noPurchases: { total: number; of: number; items?: HealthNoPurchase[] };
  malformedRecipes: { total: number; items?: HealthMalformed[] };
  pendingRecipes: { undecided: HealthPendingCount; review: HealthPendingCount };
}
export interface HealthQuery {
  from?: string;
  to?: string;
  section?: HealthSection;
  limit?: number;
  offset?: number;
  noPurchasesSince?: string;
}

export const backofficeApi = {
  // Cajas (catálogo). `cashRegisters` = activas + estado de sesión; `allCashRegisters` = gestión.
  cashRegisters: () => api.get<{ items: CashRegister[] }>('/cash-registers'),
  allCashRegisters: () => api.get<{ items: CashRegisterConfig[] }>('/cash-registers/all'),
  // null quita el fondo fijo: el cierre vuelve a pedir teclearlo.
  setFixedFloat: (id: number, fixedFloat: number | null) =>
    api.put<CashRegisterConfig>(`/cash-registers/${id}/fixed-float`, { fixedFloat }),
  createCashRegister: (name: string) => api.post<CashRegister>('/cash-registers', { name }),
  updateCashRegister: (id: number, b: { name: string; isActive: boolean }) =>
    api.patch<CashRegister>(`/cash-registers/${id}`, b),

  cashCurrent: (registerId: number) => api.get<CashSession | null>(`/cash-sessions/current?registerId=${registerId}`),
  // Qué piezas se pueden contar. La moneda es un parámetro de frontera: el servidor rechaza una
  // desconocida en vez de caer a MXN, así que no se manda vacía por si acaso.
  cashDenominations: (currency?: string) =>
    api.get<{ items: Denomination[] }>(
      currency ? `/cash/denominations?currency=${encodeURIComponent(currency)}` : '/cash/denominations'),
  cashOpen: (registerId: number, apertura: AperturaInput) =>
    api.post<CashSession>('/cash-sessions', { registerId, ...apertura }),
  // El efectivo va contado (`counts`) o como cifra en `declared` con su `manualReason`, nunca las
  // dos: el servidor rechaza la ambigüedad (FR-015).
  // `declared` lleva SOLO los métodos cuyo dinero no está en el cajón: el del cajón se declara una
  // vez, contándolo (`counts`) o con su cifra y motivo (`countedCash` + `manualReason`). Mandar un
  // método de cajón en `declared` lo rechaza el servidor nombrándolo.
  cashClose: (registerId: number, declared: Record<string, number>, extra?: {
    counts?: { denominationId: number; pieces: number }[];
    countedCash?: number;
    manualReason?: string;
    notes?: string;
    tipsDecision?: TipsDecision;
    terminalCounts?: Record<string, number>;
    floatLeft?: number;
    fixedFloatShown?: number;
    withdrawalUserId?: number;
    // Cerrar sin la salida de quien abrió (spec 046). Solo con el permiso; el servidor lo ignora si
    // ya no hace falta.
    authorizedClose?: { reason: string };
  }) => api.post<CashSession>('/cash-sessions/close', { registerId, declared, ...extra }),
  // Qué pide la asistencia para abrir o cerrar esa caja (spec 046). Lo calcula el servidor con la
  // misma regla que aplica al confirmar.
  cashAttendance: (registerId: number) =>
    api.get<CashAttendanceGate>(`/cash-sessions/attendance?registerId=${registerId}`),
  cashTips: (registerId: number) => api.get<TipsPending>(`/cash-sessions/tips?registerId=${registerId}`),
  cashTipPayout: (registerId: number, input: TipPayoutInput) =>
    api.post<{ items: { movementId: number; recipientName: string; amount: string }[] }>(
      '/cash-sessions/tips/payouts', { registerId, ...input }),
  // Paginado y con rango opcional por día del turno: sin páginas solo existían los 50 más recientes.
  cashHistory: (p: { page: number; pageSize: number; from?: string; to?: string }) => {
    const q = new URLSearchParams({ page: String(p.page), pageSize: String(p.pageSize) });
    if (p.from) q.set('from', p.from);
    if (p.to) q.set('to', p.to);
    return api.get<{ items: CashSessionRow[]; total: number; page: number; pageSize: number }>(`/cash-sessions?${q}`);
  },
  cashSession: (id: number) => api.get<CashSessionDetail>(`/cash-sessions/${id}`),
  // Las ventas de un corte más allá de la primera página. El detalle trae las primeras; esto existe
  // para poder llegar al resto — un arqueo cuyas ventas no se pueden recorrer no se puede auditar.
  cashSessionSales: (id: number, page: number, pageSize: number) =>
    api.get<{ items: CorteSale[]; total: number; salesTotal: string }>(
      `/cash-sessions/${id}/sales?page=${page}&pageSize=${pageSize}`),
  cashMovement: (registerId: number, input: CashMovementInput) =>
    api.post<CashSession>('/cash-sessions/movements', { registerId, ...input }),
  // Conceptos de salida (spec 032): la lista, y el alta en línea desde la captura.
  cashConcepts: () => api.get<{ items: CashConcept[] }>('/cash-concepts'),
  createCashConcept: (name: string) => api.post<CashConcept>('/cash-concepts', { name }),
  updateCashConcept: (id: number, body: { name: string; categoryId: number | null; supplierId: number | null } | { archived: true }) =>
    api.patch<CashConcept | null>(`/cash-concepts/${id}`, body),
  mergeCashConcept: (id: number, intoId: number) => api.post<null>(`/cash-concepts/${id}/merge`, { intoId }),
  summaryEmails: () => api.get<{ emails: string[] }>('/settings/daily-summary-emails'),
  setSummaryEmails: (emails: string[]) => api.put<{ emails: string[] }>('/settings/daily-summary-emails', { emails }),
  cardTerminals: () => api.get<{ items: CardTerminal[] }>('/card-terminals'),
  createCardTerminal: (branchId: number, name: string) => api.post<CardTerminal>('/card-terminals', { branchId, name }),
  updateCardTerminal: (id: number, body: { name: string } | { archived: true }) =>
    api.patch<CardTerminal | null>(`/card-terminals/${id}`, body),
  cardCountModes: () => api.get<{ items: { branchId: number; name: string; mode: 'auto' | 'per_terminal' }[] }>('/branches/card-count-modes'),
  setCardCountMode: (branchId: number, mode: 'auto' | 'per_terminal') =>
    api.put<null>(`/branches/${branchId}/card-count-mode`, { mode }),
  correctCashOut: (movementId: number, input: { amount: number; conceptId: number }) =>
    api.post<CashSession>(`/cash-movements/${movementId}/correct`, input),
  // Traspaso de efectivo entre dos cajas abiertas (genera salida en origen + entrada en destino).
  cashTransfer: (fromRegisterId: number, toRegisterId: number, amount: number, note?: string) =>
    api.post<{ id: number }>('/cash-sessions/transfer', { fromRegisterId, toRegisterId, amount, note }),
  // Config de negocio (admin/gerente): qué método se declara solo al cerrar caja.
  // Los tres interruptores de un método, todos OPCIONALES: lo que no se manda, no cambia. Con un
  // booleano obligatorio, apagar uno apagaría los otros — y el del cajón mueve dinero.
  updatePaymentMethod: (id: number, flags: {
    autoDeclare?: boolean; isActive?: boolean; affectsCashDrawer?: boolean;
  }) => api.patch<PaymentMethod>(`/payment-methods/${id}`, flags),

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
    // Solo compras con algo que no ha llegado (spec 038).
    pending?: boolean;
  }) => {
    const q = new URLSearchParams();
    if (params?.status) q.set('status', params.status);
    if (params?.pending) q.set('pending', 'true');
    if (params?.page != null) q.set('page', String(params.page));
    if (params?.pageSize != null) q.set('pageSize', String(params.pageSize));
    if (params?.sort) { q.set('sort', params.sort); q.set('dir', params.dir ?? 'asc'); }
    const qs = q.toString();
    return api.get<{ items: Expense[]; total: number; page: number; pageSize: number }>(`/expenses${qs ? `?${qs}` : ''}`);
  },
  createExpense: (b: {
    expenseDate?: string; // fecha del documento
    expenseDay?: string;  // día del gasto, solo sin caja abierta (spec 032)
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
  // El conjunto deseado de renglones de almacén: el servidor mueve el almacén por la diferencia.
  saveStockLines: (id: number, lines: StockLineBody[]) =>
    api.put<ExpenseDetail>(`/expenses/${id}/stock-lines`, { lines }),
  updateInventorySettings: (b: { costingMethod: CostingMethod; stockItemLabel: string }) =>
    api.put<unknown>('/settings/inventory', b),
  // «Por recibir» (spec 038): lo pagado que no ha llegado, renglón por renglón.
  pendingReceipts: () => api.get<PendingReceipt[]>('/stock/pending-receipts'),
  receiveItem: (expenseId: number, itemId: number, b: { qty: string; seenApplied: string; extraAmount?: string }) =>
    api.post<void>(`/expenses/${expenseId}/items/${itemId}/receive`, b),
  markNotComing: (expenseId: number, itemId: number, qty: string) =>
    api.post<void>(`/expenses/${expenseId}/items/${itemId}/not-coming`, { qty }),
  addArrivedItem: (expenseId: number, b: ArrivedItemBody) => api.post<void>(`/expenses/${expenseId}/items`, b),
  // Conteo (spec 038). Las cifras van en la unidad base del insumo.
  // null sin conteo abierto (204): TanStack Query no acepta `undefined` como dato.
  currentCount: () => api.get<StockCount | undefined>('/stock/counts/current').then((c) => c ?? null),
  openCount: () => api.post<StockCount>('/stock/counts', {}),
  captureCount: (id: number, ingredientId: number, body: CountCaptureBody) =>
    api.put<{ countedQty: string }>(`/stock/counts/${id}/lines/${ingredientId}`, body),
  uncaptureCount: (id: number, ingredientId: number) => api.del<void>(`/stock/counts/${id}/lines/${ingredientId}`),
  previewCount: (id: number) => api.get<CountClose>(`/stock/counts/${id}/preview`),
  closeCount: (id: number) => api.post<CountClose>(`/stock/counts/${id}/close`, {}),
  discardCount: (id: number) => api.post<void>(`/stock/counts/${id}/discard`, {}),
  lastClosedCount: () => api.get<{ closedAt: string | null; id: number | null }>('/stock/counts/last-closed'),
  // Lugares del recorrido del conteo (spec 040): una lista por empresa en vez de texto libre.
  setCountPlace: (ingredientId: number, placeId: number | null) => api.put<void>(`/stock/ingredients/${ingredientId}/place`, { placeId }),
  countPlaces: () => api.get<CountPlace[]>('/stock/count-places'),
  createCountPlace: (name: string) => api.post<{ id: number }>('/stock/count-places', { name }),
  renameCountPlace: (id: number, name: string) => api.put<void>(`/stock/count-places/${id}`, { name }),
  moveCountPlace: (id: number, toPosition: number) => api.post<void>(`/stock/count-places/${id}/move`, { toPosition }),
  archiveCountPlace: (id: number, moveTo?: number | null) =>
    api.post<void>(`/stock/count-places/${id}/archive`, moveTo === undefined ? {} : { moveTo }),

  // Extracción del documento → borrador. NO escribe nada: el operador confirma.
  parseDoc: (file: File) => {
    const fd = new FormData();
    fd.append('file', file);
    return api.postForm<ParsedDoc>('/expenses/parse-doc', fd);
  },

  // Artículos del almacén (spec 042): la lista con filtros, la ficha, su historial y su bitácora.
  articles: (f: ArticleFilter = {}) => {
    const qs = new URLSearchParams();
    if (f.q?.trim()) qs.set('q', f.q.trim());
    if (f.archived) qs.set('status', 'archived');
    if (f.place === 'none') qs.set('placeId', 'none');
    else if (typeof f.place === 'number') qs.set('placeId', String(f.place));
    if (f.noCost) qs.set('noCost', '1');
    if (f.negative) qs.set('negative', '1');
    const q = qs.toString();
    return api.get<ArticleRow[]>(`/stock/articles${q ? `?${q}` : ''}`);
  },
  createArticle: (b: ArticleCreate) => api.post<{ id: number }>('/stock/articles', b),
  article: (id: number) => api.get<ArticleSheet>(`/stock/articles/${id}`),
  // Solo los campos que vienen cambian; uno en null se limpia (spec 043). Devuelve la versión nueva.
  updateArticle: (id: number, b: ArticleUpdate) => api.patch<{ updatedAt: string }>(`/stock/articles/${id}`, b),
  correctArticleCost: (id: number, cost: string) => api.post<void>(`/stock/articles/${id}/cost`, { cost }),
  articleCostHistory: (id: number, before?: string) =>
    api.get<CostHistoryPage>(`/stock/articles/${id}/cost-history${before ? `?before=${encodeURIComponent(before)}` : ''}`),
  previewUnitChange: (id: number, b: UnitChangeBody) => api.post<UnitChangePreview>(`/stock/articles/${id}/unit/preview`, b),
  changeUnit: (id: number, b: UnitChangeBody) => api.post<void>(`/stock/articles/${id}/unit`, b),
  articleArchiveCheck: (id: number) => api.get<ArchiveCheck>(`/stock/articles/${id}/archive-check`),
  archiveArticle: (id: number) => api.post<void>(`/stock/articles/${id}/archive`, {}),
  reactivateArticle: (id: number) => api.post<void>(`/stock/articles/${id}/reactivate`, {}),
  presentationSuggestions: (unitKind: string) =>
    api.get<{ name: string; qtyInBase: string }[]>(`/stock/presentation-suggestions?unitKind=${encodeURIComponent(unitKind)}`),
  articleCategories: () => api.get<{ id: number; name: string }[]>('/stock/ingredient-categories'),
  createArticleCategory: (name: string) => api.post<{ id: number; name: string }>('/stock/ingredient-categories', { name }),
  articleAudit: (id: number, before?: string) =>
    api.get<ArticleAuditPage>(`/stock/articles/${id}/audit${before ? `?before=${encodeURIComponent(before)}` : ''}`),

  // Catálogo de artículos (insumos) y buscador del detalle del gasto.
  units: () => api.get<{ items: Unit[] }>('/units'),
  ingredients: (onlyActive = false) =>
    api.get<{ items: Ingredient[] }>(`/ingredients${onlyActive ? '?onlyActive=true' : ''}`),
  createIngredient: (b: { name: string; baseUnitId: number; minStock?: string }) =>
    api.post<Ingredient>('/ingredients', b),
  presentations: () => api.get<Presentation[]>('/stock/presentations'),
  createPresentation: (ingredientId: number, b: PresentationBody) =>
    api.post<Presentation>(`/stock/ingredients/${ingredientId}/presentations`, b),
  updatePresentation: (id: number, b: PresentationBody) => api.put<Presentation>(`/stock/presentations/${id}`, b),
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
  inventoryHealth: (q: HealthQuery = {}) => api.get<InventoryHealth>(`/stock/health?${qsReporte(q as ReportQuery)}`),
  // El archivo de la misma pantalla. El nombre viene del servidor (Content-Disposition), con el periodo.
  exportInventoryHealth: async (q: Pick<HealthQuery, 'from' | 'to'>) => {
    const res = await api.getRaw(`/stock/health.csv?${qsReporte(q as ReportQuery)}`);
    const name = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') ?? '')?.[1] ?? 'salud-del-inventario.csv';
    return { blob: await res.blob(), filename: name };
  },
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
      // uncosted_revenue: lo vendido sin costo capturado; NO está en margin (spec 029).
      items: Array<{ product_name: string; qty: string; revenue: string; cost: string; margin: string; uncosted_revenue?: string }>;
    }>(`/reports/margins?${qsReporte({ ...q, limit: 50 })}`),
  // Unidades por producto, sueltas y dentro de paquetes.
  reportProductsSold: (q: ReportQuery = {}) =>
    api.get<{
      range: ReportRange;
      items: Array<{ product_name: string; alone: string; in_packages: string }>;
    }>(`/reports/products-sold?${qsReporte({ ...q, limit: 50 })}`),
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

// Artículos del almacén (spec 042). Cantidades en la unidad base del artículo.
export interface ArticleFilter {
  q?: string;
  archived?: boolean;
  place?: number | 'none';
  noCost?: boolean;
  negative?: boolean;
}
export interface PresentationLabel { id: number; name: string; qtyInBase: string }
export interface ArticleRow {
  id: number;
  name: string;
  isActive: boolean;
  baseUnitId: number;
  baseUnitCode: string;
  baseUnitKind: string;
  currentCost: string;
  minStock: string | null;
  isPrep: boolean;
  categoryId: number | null;
  category: string | null;
  countPlaceId: number | null;
  countPlace: string | null;
  onHand: string;
  // Opcional a propósito: el compilador obliga a la guarda aunque el servidor siempre mande [] (AGENTS §1).
  presentations?: PresentationLabel[];
  // «estimated»: su receta vino estimada y está por revisar.
  compositionStatus?: string;
}
export interface ArticleSheet extends ArticleRow {
  costSource: string;
  updatedAt: string;
}
export interface ArticleCreate {
  name: string;
  baseUnitId: number;
  categoryId?: number | null;
  placeId?: number | null;
  minStock?: string;
  initialCost?: string;
  initialStock?: string;
}
export interface ArticleUpdate {
  expectedUpdatedAt: string;
  name?: string;
  categoryId?: number | null;
  placeId?: number | null;
  minStock?: string | null;
}
export interface CostHistoryItem {
  kind: 'purchase' | 'correction';
  at: string;
  id: number;
  unitCost: string;
  presentation: string | null;
  packQty: string | null;
  packCost: string | null;
  supplier: string | null;
  user: string | null;
}
export interface CostHistoryPage { items?: CostHistoryItem[]; next: string | null; trendPct: string | null }
export interface ArticleAuditItem {
  id: number;
  at: string;
  action: string;
  entity: string;
  actor: string | null;
  before: Record<string, unknown> | null;
  after: Record<string, unknown> | null;
  // Cuántos guardados seguidos junta la entrada (spec 043); falta en una API anterior.
  changes?: number;
}
export interface ArticleAuditPage { items?: ArticleAuditItem[]; next: string | null }
export interface ArchiveCheck {
  onHand?: { branch: string; qty: string }[];
  activeRecipes?: { kind: 'product' | 'option' | 'ingredient'; id: number; name: string }[];
  openCount: boolean;
  pendingReceipts: number;
}
export interface UnitChangeBody {
  baseUnitId: number;
  factor: string;
  overrides?: Record<string, string>;
}
export interface UnitConversion {
  key: string;
  group: 'stock' | 'recipe' | 'presentation';
  label: string;
  from: string;
  to: string;
  fits: boolean;
}
export interface UnitChangePreview {
  path: 'direct' | 'replace';
  fromUnit: string;
  toUnit: string;
  conversions?: UnitConversion[];
}
