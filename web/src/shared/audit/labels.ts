import { money } from '../../utils/format';

// Cómo se lee cada acción de la bitácora. Las claves son las del servidor (domain.AllAuditActions)
// y nunca salen a pantalla: son texto para quien programa.
export const ACTION_LABELS: Record<string, string> = {
  'order.created': 'Pedido creado',
  'order.lines_added': 'Productos agregados',
  'order.line_removed': 'Producto quitado',
  'order.lines_moved': 'Productos pasados a otro pedido',
  'order.delivered': 'Entregado',
  'order.cancelled': 'Pedido cancelado',
  'order.discount_set': 'Descuento',
  'order.platform_ref_set': 'Folio de plataforma',
  'payment.charged': 'Cobrado',
  'payment.voided': 'Pago devuelto',
  'payment.refunded': 'Devolución',
  'stock.written_off': 'Dado por perdido',
  'cash.opened': 'Caja abierta',
  'cash.closed': 'Caja cerrada',
  'cash.closed_without_opener_clock_out': 'Caja cerrada sin la salida de quien la abrió',
  'cash.movement': 'Movimiento de efectivo',
  'cash.movement_corrected': 'Salida corregida',
  'expense.created': 'Gasto registrado',
  'expense.paid': 'Gasto pagado',
  'expense.cancelled': 'Gasto cancelado',
  'tip.paid_out': 'Propina entregada',
  'price.set': 'Precio cambiado',
  'price.deleted': 'Precio quitado',
  'price.synced': 'Precio de plataforma actualizado',
  'attendance.clock_in': 'Entrada registrada',
  'attendance.clock_out': 'Salida registrada',
  'attendance.corrected': 'Registro corregido',
  'attendance.manual_entry': 'Registro capturado a mano',
  'attendance.settings_changed': 'Ajustes de asistencia',
  'attendance.schedule_set': 'Horario capturado',
  'user.tracks_attendance_changed': 'Lleva asistencia',
  'payment.terminal_changed': 'Terminal cambiada',
  'product.consumption_mode': 'Cambió qué resta',
  'expense.received': 'Llegó mercancía',
  'expense.count_changed': 'Cambió “En el arqueo” de un pago',
  'cash.opening_backdated': 'Abrió la caja con hora anterior',
  'expense.stock_corrected': 'Corrigió lo que llegó',
  'stock.counted': 'Cerró un conteo',
  'settings.costing_method': 'Cambió el costeo del almacén',
  'waste.recorded': 'Se echó a perder',
  'waste.approved': 'Aprobó lo que se echó a perder',
  'waste.rejected': 'Rechazó lo que se echó a perder',
  'waste.undone': 'Deshizo lo que se echó a perder',
  'settings.waste_threshold': 'Cambió el tope de aprobación',
  'settings.count_limits_changed': 'Cambió los límites del conteo',
  'ingredient.created': 'Creó el artículo',
  'ingredient.updated': 'Editó el artículo',
  'ingredient.archived': 'Archivó el artículo',
  'ingredient.reactivated': 'Reactivó el artículo',
  'ingredient.cost_corrected': 'Corrigió el costo',
  'ingredient.unit_changed': 'Cambió en qué se mide',
  'ingredient.unit_changed.detail': 'Detalle del cambio de unidad',
  'ingredient.replaced': 'Reemplazó el artículo por otro',
  'presentation.created': 'Agregó un empaque',
  'presentation.updated': 'Editó un empaque',
  'presentation.retired': 'Retiró o volvió un empaque',
  'stock.count_confirm_asked': 'El conteo pidió confirmar una cifra',
};

// Qué resta un producto (spec 037), con las palabras de la hoja de receta.
const MODE_LABELS: Record<string, string> = {
  own_stock: 'Su propia existencia', recipe: 'Lo que lleva', package: 'Es un combo',
  none: 'No se cuenta', undecided: 'Falta decir qué resta',
};

export function actionLabel(action: string): string {
  return ACTION_LABELS[action] ?? 'Otra acción';
}

// Los campos que se muestran, con su nombre y si son dinero. Lo demás (ids internos) no se pinta.
const FIELDS: Array<[string, string, boolean]> = [
  ['terminalName', 'Terminal', false],
  ['discount', 'Descuento', true],
  ['price', 'Precio', true],
  ['amount', 'Monto', true],
  ['tip', 'Propina', true],
  ['total', 'Total', true],
  ['paid', 'Pagado', true],
  ['status', 'Estado', false],
  ['ref', 'Folio', false],
  ['reason', 'Motivo', false],
  ['mode', 'Qué resta', false],
];

type ChangeData = Record<string, unknown> | null | undefined;

function formatValue(v: unknown, isMoney: boolean, key: string): string {
  // Un pago con tarjeta sin terminal es un hecho que se corrige, no un dato que falta.
  if ((v === null || v === undefined || v === '') && key === 'terminalName') return 'Sin terminal';
  if (v === null || v === undefined || v === '') return '—';
  if (key === 'mode') return MODE_LABELS[String(v)] ?? String(v);
  return isMoney ? money(String(v)) : String(v);
}

// summarizeChange pinta el «antes → después» de un evento en una línea: «Precio: $80.00 → $85.00».
export function summarizeChange(before: ChangeData, after: ChangeData): string {
  const parts: string[] = [];
  for (const [k, name, isMoney] of FIELDS) {
    const a = before?.[k];
    const d = after?.[k];
    if (a === undefined && d === undefined) continue;
    // Lo que no cambió (el monto de un pago al cambiarle la terminal) se dice una vez.
    parts.push(a !== undefined && d !== undefined && a !== d
      ? `${name}: ${formatValue(a, isMoney, k)} → ${formatValue(d, isMoney, k)}`
      : `${name}: ${formatValue(d ?? a, isMoney, k)}`);
  }
  return parts.join(' · ');
}
