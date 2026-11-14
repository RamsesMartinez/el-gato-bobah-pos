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
  'expense.edited': 'Gasto editado',
  'stock.counted': 'Cerró un conteo',
  'settings.costing_method': 'Cambió el costeo del almacén',
  'waste.recorded': 'Se echó a perder',
  'waste.approved': 'Aprobó lo que se echó a perder',
  'waste.rejected': 'Rechazó lo que se echó a perder',
  'waste.undone': 'Deshizo lo que se echó a perder',
  'stock.out_reported': 'Avisó que se acabó',
  'stock.out_undone': 'Deshizo un «se acabó»',
  'stock.low_stock_configured': 'Cambió las alertas de stock bajo',
  'notification.recipients_changed': 'Cambió quién recibe los avisos',
  'notification.resolved': 'Cerró un aviso',
  'settings.waste_threshold': 'Cambió el tope de aprobación',
  'settings.count_limits_changed': 'Cambió los límites del conteo',
  'ingredient.created': 'Creó el artículo',
  'ingredient.updated': 'Editó el artículo',
  'ingredient.archived': 'Archivó el artículo',
  'ingredient.reactivated': 'Reactivó el artículo',
  'ingredient.cost_corrected': 'Corrigió el costo',
  'ingredient.unit_changed': 'Cambió en qué se mide',
  'ingredient.unit_changed.detail': 'Detalle del cambio de unidad',
  'ingredient.density_set': 'Capturó la densidad',
  'ingredient.density_cleared': 'Quitó la densidad',
  'presentation.tare_set': 'Cambió lo que pesa vacío un empaque',
  'ingredient.replaced': 'Reemplazó el artículo por otro',
  'ingredient.merged': 'Unió otros artículos en éste',
  'ingredient.merged_into': 'Se unió a otro artículo',
  'presentation.created': 'Agregó un empaque',
  'presentation.updated': 'Editó un empaque',
  'presentation.retired': 'Retiró o volvió un empaque',
  'stock.count_confirm_asked': 'El conteo pidió confirmar una cifra',
  'role.created': 'Creó un rol',
  'role.renamed': 'Cambió el nombre de un rol',
  'role.permissions_changed': 'Cambió lo que puede hacer un rol',
  'role.reset': 'Regresó un rol a lo sugerido',
  'role.deactivated': 'Inactivó un rol',
  'role.reactivated': 'Reactivó un rol',
  'role.deleted': 'Borró un rol',
  'user.role_changed': 'Cambió el rol de una persona',
  'api_token.created': 'Creó un token para un asistente',
  'api_token.revoked': 'Revocó un token de asistente',
  'mcp.tool_called': 'Un asistente usó el POS',
};

// Qué resta un producto (spec 037), con las palabras de la hoja de receta.
const MODE_LABELS: Record<string, string> = {
  own_stock: 'Su propia existencia', options: 'Lo restan sus opciones', recipe: 'Lo que lleva', package: 'Es un combo',
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

// ChangeContext trae lo que la bitácora no guarda con palabras: la etiqueta de cada permiso y el
// nombre de cada rol. Sin ellos se dice cuántos, nunca el código.
export interface ChangeContext {
  permissionLabel?: (code: string) => string | undefined;
  roleName?: (id: number) => string | undefined;
}

const strings = (v: unknown): string[] => (Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []);

function permissionList(codes: string[], ctx: ChangeContext): string {
  const labels = codes.map((c) => ctx.permissionLabel?.(c));
  if (labels.every((l): l is string => !!l)) return labels.join(', ');
  return `${codes.length} ${codes.length === 1 ? 'permiso' : 'permisos'}`;
}

// summarizeEvent es summarizeChange con lo propio de cada acción: los eventos de roles guardan
// nombres y listas de permisos que la lista de campos genérica no sabe leer.
export function summarizeEvent(action: string, before: ChangeData, after: ChangeData, ctx: ChangeContext = {}): string {
  if (action === 'user.role_changed') {
    const from = typeof before?.roleId === 'number' ? ctx.roleName?.(before.roleId) : undefined;
    const to = typeof after?.roleId === 'number' ? ctx.roleName?.(after.roleId) : undefined;
    return from && to ? `Rol: ${from} → ${to}` : 'Cambió de rol';
  }
  if (!action.startsWith('role.')) return summarizeChange(before, after);
  const parts: string[] = [];
  const name = (after?.name ?? before?.name) as string | undefined;
  if (action === 'role.renamed' && before?.name && after?.name) parts.push(`Rol: ${before.name} → ${after.name}`);
  else if (name) parts.push(`Rol: ${name}`);
  const perms = strings(after?.permissions);
  if (action === 'role.created' && perms.length) parts.push(`Puede: ${permissionList(perms, ctx)}`);
  const added = strings(after?.added);
  const removed = strings(after?.removed);
  if (added.length) parts.push(`Agregó: ${permissionList(added, ctx)}`);
  if (removed.length) parts.push(`Quitó: ${permissionList(removed, ctx)}`);
  return parts.join(' · ');
}

// auditGroupOf es el grupo de cada acción, espejo de domain.AuditGroupOf: solo pinta el punto de color.
export function auditGroupOf(action: string): 'money' | 'cancel' | 'price' | 'cash' | 'attendance' {
  if (['payment.voided', 'payment.refunded', 'order.cancelled', 'order.line_removed', 'stock.written_off'].includes(action)) return 'cancel';
  if (action.startsWith('price.') || action.startsWith('product.')) return 'price';
  if (action.startsWith('cash.') || action.startsWith('expense.') || action.startsWith('tip.')) return 'cash';
  if (action.startsWith('attendance.') || action.startsWith('user.') || action.startsWith('role.')) return 'attendance';
  return 'money';
}
