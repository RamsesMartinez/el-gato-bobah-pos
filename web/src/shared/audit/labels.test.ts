import { expect, test } from 'vitest';
import { ACTION_LABELS, actionLabel, summarizeChange } from './labels';

// La lista del servidor (domain.AllAuditActions). Una acción sin etiqueta saldría en pantalla como
// «payment.voided», que es texto para quien programa.
const FROM_SERVER = [
  'order.created', 'order.lines_added', 'order.line_removed', 'order.lines_moved', 'order.delivered',
  'order.cancelled', 'order.discount_set', 'order.platform_ref_set', 'payment.charged',
  'payment.voided', 'payment.refunded', 'stock.written_off', 'cash.opened', 'cash.closed',
  'cash.movement', 'cash.movement_corrected', 'expense.created', 'expense.paid', 'expense.cancelled', 'tip.paid_out',
  'expense.received', 'expense.stock_corrected', 'stock.counted', 'settings.costing_method',
  'price.set', 'price.deleted', 'price.synced', 'attendance.clock_in', 'attendance.clock_out',
  'attendance.corrected', 'attendance.manual_entry', 'attendance.settings_changed',
  'attendance.schedule_set', 'user.tracks_attendance_changed', 'payment.terminal_changed',
  'product.consumption_mode',
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
