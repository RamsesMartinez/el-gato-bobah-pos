import type {
  DraftDiscount, DraftModifierView, DraftView, OrderView, ServiceType, TicketModifier,
} from '../../types/pos';
import { round2 } from '../../domain/numeros';

// Lo que el ticket pinta de la cuenta abierta, armado con lo que dijo el servidor.
//
// Es una función pura para que la regla de qué va en cada sección —Nuevo, En cocina, Pagado— tenga
// una sola copia: la usan el panel, la píldora y la barra angosta, y tres copias ya divergieron una
// vez con el total del envío.

export interface RenglonNuevo {
  id: string;
  productId: number;
  name: string;
  qty: number;
  modifiers: DraftModifierView[];
  notes: string;
  lineTotal: number;
  available: boolean;
  // El servidor todavía no lo confirma: se ve, pero no se puede mandar ni cobrar.
  guardando: boolean;
  version: number;
}

export interface RenglonPedido {
  id: number;
  name: string;
  detalle: string;
  qty: number;
  delivered: number;
  lineTotal: number;
}

// Un toque que el servidor todavía no confirma.
export interface Pendiente {
  opId: string;
  cuenta: string;
  productId: number;
  name: string;
  qty: number;
  unitPrice: number;
  modifiers: TicketModifier[];
  notes: string;
}

export interface VistaCuenta {
  tipo: 'nueva' | 'captura' | 'pedido';
  borradorId: string | null;
  pedidoId: number | null;
  nombre: string;
  numero: number | null;
  // Cuándo se abrió: la del pedido, o la de la cuenta en captura.
  abiertaEn: string | null;
  customerName: string;
  serviceType: ServiceType;
  platformId: number | null;
  platformOrderRef: string;
  deliveryFee: string;
  discount: DraftDiscount | null;
  nuevos: RenglonNuevo[];
  enCocina: RenglonPedido[];
  pagados: RenglonPedido[];
  subtotalNuevo: number;
  descuentoNuevo: number;
  totalNuevo: number;
  totalPedido: number;
  pagado: number;
  falta: number;
  noDisponibles: RenglonNuevo[];
  guardando: boolean;
  esPlataforma: boolean;
  // Pagada completa Y entregada: ya no recibe productos (D-9).
  cerrada: boolean;
  pedido: OrderView | null;
}

export interface CabeceraLocal {
  serviceType: ServiceType;
  platformId: number | null;
  platformOrderRef: string;
  customerName: string;
}

function renglonesNuevos(draft: DraftView | undefined, pendientes: Pendiente[]): RenglonNuevo[] {
  const delServidor: RenglonNuevo[] = (draft?.lines ?? []).map((l) => ({
    id: l.id, productId: l.productId, name: l.productName, qty: Number(l.qty),
    modifiers: l.modifiers ?? [], notes: l.notes ?? '', lineTotal: Number(l.lineTotal),
    available: l.available, guardando: false, version: l.version,
  }));
  const enVuelo: RenglonNuevo[] = pendientes.map((p) => ({
    id: p.opId, productId: p.productId, name: p.name, qty: p.qty,
    modifiers: p.modifiers.map((m) => ({ optionId: m.optionId, name: m.name, qty: m.qty, priceDelta: String(m.priceDelta) })),
    notes: p.notes, available: true, guardando: true, version: 0,
    lineTotal: round2((p.unitPrice + p.modifiers.reduce((s, m) => s + m.priceDelta * m.qty, 0)) * p.qty),
  }));
  return [...delServidor, ...enVuelo];
}

export function detalleDeModificadores(mods: Array<{ name: string; quantity?: number; qty?: number }>): string {
  return mods.map((m) => {
    const n = m.quantity ?? m.qty ?? 1;
    return n > 1 ? `${m.name} ×${n}` : m.name;
  }).join(' · ');
}

// Lo pagado de cada renglón sale de los pagos vivos: un renglón cubierto entero lleva candado.
function piezasPagadas(order: OrderView): Map<number, number> {
  const m = new Map<number, number>();
  for (const p of order.payments ?? []) {
    if (p.voided) continue;
    for (const l of p.lines ?? []) m.set(l.lineId, (m.get(l.lineId) ?? 0) + Number(l.qty));
  }
  return m;
}

export function armarVista(args: {
  draft?: DraftView;
  order?: OrderView;
  pendientes: Pendiente[];
  cabeceraNueva: CabeceraLocal;
  guardando: boolean;
}): VistaCuenta {
  const { draft, order, pendientes, cabeceraNueva, guardando } = args;
  const nuevos = renglonesNuevos(draft, pendientes);
  const enCocina: RenglonPedido[] = [];
  const pagados: RenglonPedido[] = [];
  if (order) {
    const pagadas = piezasPagadas(order);
    for (const l of order.lines ?? []) {
      if (l.cancelled || l.id === undefined) continue;
      const r: RenglonPedido = {
        id: l.id, name: l.productName, detalle: detalleDeModificadores(l.modifiers ?? []),
        qty: Number(l.quantity), delivered: Number(l.delivered), lineTotal: Number(l.lineTotal),
      };
      if ((pagadas.get(l.id) ?? 0) >= r.qty) pagados.push(r);
      else enCocina.push(r);
    }
  }
  const totalPendientes = round2(nuevos.filter((n) => n.guardando).reduce((s, n) => s + n.lineTotal, 0));
  const totalNuevo = round2(Number(draft?.total ?? 0) + totalPendientes);
  const totalPedido = Number(order?.total ?? 0);
  const faltaPedido = Number(order?.outstanding ?? 0);
  const tipo: VistaCuenta['tipo'] = order ? 'pedido' : draft ? 'captura' : 'nueva';
  const head = order
    ? {
      serviceType: order.serviceType as ServiceType, platformId: order.deliveryPlatformId,
      platformOrderRef: order.platformOrderRef ?? '', customerName: order.customerName ?? '',
    }
    : draft
      ? {
        serviceType: draft.serviceType, platformId: draft.platformId,
        platformOrderRef: draft.platformOrderRef ?? '', customerName: draft.customerName ?? '',
      }
      : cabeceraNueva;
  return {
    tipo,
    borradorId: draft?.id ?? null,
    pedidoId: order?.id ?? null,
    nombre: order?.folioName || draft?.folioName || '',
    numero: order?.number ?? null,
    abiertaEn: order?.openedAt || draft?.createdAt || null,
    ...head,
    deliveryFee: draft?.deliveryFee ?? order?.deliveryFee ?? '0.00',
    discount: draft?.discount ?? null,
    nuevos,
    enCocina,
    pagados,
    subtotalNuevo: round2(Number(draft?.subtotal ?? 0) + totalPendientes),
    descuentoNuevo: Number(draft?.discountTotal ?? 0),
    totalNuevo,
    totalPedido,
    pagado: round2(totalPedido - faltaPedido),
    falta: round2(faltaPedido + totalNuevo),
    noDisponibles: nuevos.filter((n) => !n.available),
    guardando: guardando || nuevos.some((n) => n.guardando),
    esPlataforma: head.platformId !== null,
    cerrada: order !== undefined && order.status === 'entregada' && order.paid,
    pedido: order ?? null,
  };
}
