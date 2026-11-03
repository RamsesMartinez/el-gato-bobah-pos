import { useSessionStore, type SessionUser } from '../stores/session';

// Los permisos que la pantalla consulta. Espejo de `domain.Permission` del servidor; la barrera
// sigue siendo `RequirePermission` allá, esto solo evita ofrecer lo que terminaría en 403.
export type Permission = 'payments.void' | 'orders.cancel' | 'orders.move_lines' | 'orders.cancel_pending'
  | 'attendance.configure' | 'attendance.report' | 'attendance.correct_others' | 'attendance.correct_own'
  | 'audit.view' | 'payments.change_terminal' | 'cash.configure' | 'cash.close_without_clock_out' | 'cash.adjust_count'
  | 'inventory.health_view' | 'menu.configure'
  | 'inventory.receive' | 'inventory.configure' | 'inventory.count'
  | 'inventory.write_off' | 'inventory.write_off_approve' | 'inventory.view'
  | 'expenses.edit';

// can pregunta por permiso y nunca por nombre de rol: cuando cada empresa defina sus roles, lo único
// que cambia es de dónde saca el servidor la lista, y ninguna pantalla se reescribe.
//
// Sin la lista —una sesión emitida por una API anterior a los permisos— responde que no: suponer
// «todo permitido» ofrecería acciones que el servidor rebota con el cliente enfrente.
//
// Un componente que tiene que repintar al cambiar de usuario pasa el `user` que leyó del store; sin
// él se lee la sesión del momento.
export function can(permission: Permission, user: SessionUser | null = useSessionStore.getState().user): boolean {
  return user?.permissions?.includes(permission) ?? false;
}
