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
};

export function actionLabel(action: string): string {
  return ACTION_LABELS[action] ?? 'Otra acción';
}

// Los campos que se muestran, con su nombre y si son dinero. Lo demás (ids internos) no se pinta.
const FIELDS: Array<[string, string, boolean]> = [
  ['discount', 'Descuento', true],
  ['price', 'Precio', true],
  ['amount', 'Monto', true],
  ['tip', 'Propina', true],
  ['total', 'Total', true],
  ['paid', 'Pagado', true],
  ['status', 'Estado', false],
  ['ref', 'Folio', false],
  ['reason', 'Motivo', false],
];

type ChangeData = Record<string, unknown> | null | undefined;

function formatValue(v: unknown, isMoney: boolean): string {
  if (v === null || v === undefined || v === '') return '—';
  return isMoney ? money(String(v)) : String(v);
}

// summarizeChange pinta el «antes → después» de un evento en una línea: «Precio: $80.00 → $85.00».
export function summarizeChange(before: ChangeData, after: ChangeData): string {
  const parts: string[] = [];
  for (const [k, name, isMoney] of FIELDS) {
    const a = before?.[k];
    const d = after?.[k];
    if (a === undefined && d === undefined) continue;
    parts.push(a !== undefined && d !== undefined
      ? `${name}: ${formatValue(a, isMoney)} → ${formatValue(d, isMoney)}`
      : `${name}: ${formatValue(d ?? a, isMoney)}`);
  }
  return parts.join(' · ');
}
