import { describe, expect, test } from 'vitest';
import { ACTION_LABELS, actionLabel, auditGroupOf, summarizeChange, summarizeEvent } from './labels';

// La lista del servidor (domain.AllAuditActions). Una acción sin etiqueta saldría en pantalla como
// «payment.voided», que es texto para quien programa.
const FROM_SERVER = [
  'order.created', 'order.lines_added', 'order.line_removed', 'order.lines_moved', 'order.delivered',
  'order.cancelled', 'order.discount_set', 'order.platform_ref_set', 'payment.charged',
  'payment.voided', 'payment.refunded', 'stock.written_off', 'cash.opened', 'cash.closed',
  'cash.closed_without_opener_clock_out',
  'cash.movement', 'cash.movement_corrected', 'expense.created', 'expense.paid', 'expense.cancelled', 'tip.paid_out',
  'expense.received', 'expense.count_changed', 'cash.opening_backdated', 'expense.stock_corrected', 'stock.counted', 'settings.costing_method',
  'waste.recorded', 'waste.approved', 'waste.rejected', 'waste.undone', 'settings.waste_threshold',
  'price.set', 'price.deleted', 'price.synced', 'attendance.clock_in', 'attendance.clock_out',
  'attendance.corrected', 'attendance.manual_entry', 'attendance.settings_changed',
  'attendance.schedule_set', 'user.tracks_attendance_changed', 'payment.terminal_changed',
  'product.consumption_mode',
  'ingredient.created', 'ingredient.updated', 'ingredient.archived', 'ingredient.reactivated',
  'ingredient.cost_corrected', 'ingredient.unit_changed', 'ingredient.unit_changed.detail',
  'ingredient.replaced', 'presentation.created', 'presentation.updated', 'presentation.retired',
  'stock.count_confirm_asked', 'settings.count_limits_changed', 'expense.edited',
  'ingredient.merged', 'ingredient.merged_into',
  'ingredient.density_set', 'ingredient.density_cleared', 'presentation.tare_set',
  'role.created', 'role.renamed', 'role.permissions_changed', 'role.reset', 'role.deactivated',
  'role.reactivated', 'role.deleted', 'user.role_changed',
];

test('cada acción del servidor tiene etiqueta en español', () => {
  for (const a of FROM_SERVER) {
    expect(ACTION_LABELS[a], a).toBeTruthy();
    expect(actionLabel(a)).not.toContain('.');
  }
  expect(Object.keys(ACTION_LABELS).sort()).toEqual([...FROM_SERVER].sort());
});

test('una acción desconocida no tumba la pantalla', () => {
  expect(actionLabel('algo.nuevo')).toBe('Otra acción');
});

test('el cambio se lee como antes → después y con dinero', () => {
  expect(summarizeChange({ discount: '0' }, { discount: '10' })).toBe('Descuento: $0 → $10');
  expect(summarizeChange(null, { amount: '45', paid: '45' })).toContain('Monto: $45');
  expect(summarizeChange(null, { reason: 'Llegó frío' })).toBe('Motivo: Llegó frío');
  expect(summarizeChange(null, null)).toBe('');
});

test('el cambio de terminal se lee con el nombre, no con el id', () => {
  expect(summarizeChange({ terminalId: 2, terminalName: 'Terminal A' }, { terminalId: 3, terminalName: 'Terminal B' }))
    .toBe('Terminal: Terminal A → Terminal B');
  expect(summarizeChange({ terminalId: null, terminalName: null }, { terminalId: 3, terminalName: 'Terminal B' }))
    .toBe('Terminal: Sin terminal → Terminal B');
});

// Qué resta un producto (spec 037) se lee con las palabras de la hoja, no con el modo interno.
test('el cambio de modo se lee en español', () => {
  expect(summarizeChange({ mode: 'own_stock' }, { mode: 'recipe' })).toBe('Qué resta: Su propia existencia → Lo que lleva');
  expect(summarizeChange({ mode: 'recipe' }, { mode: 'undecided' })).toBe('Qué resta: Lo que lleva → Falta decir qué resta');
});

// Los eventos de roles salían como «Qué cambió: —»: el servidor guarda `name`, `permissions`,
// `added`, `removed` y `roleId`, y ninguno estaba en la lista de campos.
describe('eventos de roles', () => {
  const LABELS: Record<string, string> = { 'orders.cancel': 'Cancelar pedidos', 'cash.operate': 'Operar caja' };
  const ctx = { permissionLabel: (c: string) => LABELS[c], roleName: (id: number) => ({ 3: 'Cajero', 12: 'Encargado' } as Record<number, string>)[id] };

  test('crear nombra el rol y lo que puede', () => {
    expect(summarizeEvent('role.created', null, { name: 'Encargado', permissions: ['orders.cancel', 'cash.operate'] }, ctx))
      .toBe('Rol: Encargado · Puede: Cancelar pedidos, Operar caja');
  });
  test('renombrar dice antes → después', () => {
    expect(summarizeEvent('role.renamed', { name: 'Barista' }, { name: 'Barra' }, ctx)).toBe('Rol: Barista → Barra');
  });
  test('cambiar permisos dice qué agregó y qué quitó', () => {
    expect(summarizeEvent('role.permissions_changed', null, { name: 'Cajero', added: ['orders.cancel'], removed: ['cash.operate'] }, ctx))
      .toBe('Rol: Cajero · Agregó: Cancelar pedidos · Quitó: Operar caja');
  });
  test('sin las etiquetas de permiso, dice cuántos (nunca el código)', () => {
    const s = summarizeEvent('role.permissions_changed', null, { name: 'Cajero', added: ['orders.cancel', 'x.y'], removed: [] }, {});
    expect(s).toBe('Rol: Cajero · Agregó: 2 permisos');
  });
  test.each(['role.reset', 'role.deactivated', 'role.reactivated', 'role.deleted'])('%s nombra el rol', (a) => {
    expect(summarizeEvent(a, { name: 'Barista' }, null, ctx)).toBe('Rol: Barista');
  });
  test('el cambio de rol de una persona dice de cuál a cuál, o «Cambió de rol» si no se saben', () => {
    expect(summarizeEvent('user.role_changed', { roleId: 3 }, { roleId: 12 }, ctx)).toBe('Rol: Cajero → Encargado');
    expect(summarizeEvent('user.role_changed', { roleId: 3 }, { roleId: 12 }, {})).toBe('Cambió de rol');
  });
  test('lo demás se lee igual que antes', () => {
    expect(summarizeEvent('price.set', { price: '80' }, { price: '85' }, ctx)).toBe('Precio: $80 → $85');
  });
});

// Espejo de domain.AuditGroupOf: los role.* iban con el color de Pedidos y el servidor los cuenta en
// Asistencia, junto con user.role_changed.
test('los eventos de roles van en el grupo de Asistencia, como en el servidor', () => {
  for (const a of ['role.created', 'role.renamed', 'role.permissions_changed', 'role.reset', 'role.deactivated', 'role.reactivated', 'role.deleted', 'user.role_changed']) {
    expect(auditGroupOf(a), a).toBe('attendance');
  }
  expect(auditGroupOf('payment.charged')).toBe('money');
});
