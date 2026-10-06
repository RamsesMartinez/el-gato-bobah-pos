import { afterEach, describe, expect, test } from 'vitest';

import { useSessionStore, type SessionUser } from '../stores/session';
import { can } from './permissions';

const cajero: SessionUser = { id: 7, companyId: 2, name: 'Carlos', role: 'cajero' };

afterEach(() => useSessionStore.setState({ user: null, token: null }));

// La pantalla pregunta por PERMISO, nunca por nombre de rol: cuando cada empresa defina sus roles,
// «gerente» puede no existir y lo único que cambia es de dónde sale la lista.
describe('can', () => {
  test('sin sesión no puede nada', () => {
    expect(can('orders.cancel')).toBe(false);
  });

  // Una sesión de una API anterior a los permisos no trae el campo. Leerlo como «todo permitido»
  // ofrecería acciones que el servidor rebota con 403 frente al cliente.
  test('con la sesión sin `permissions` no puede nada, aunque el rol sea admin', () => {
    expect(can('payments.void', { ...cajero, role: 'admin' })).toBe(false);
  });

  test('responde con la lista que mandó el servidor, no con el rol', () => {
    const user = { ...cajero, permissions: ['orders.cancel_pending', 'orders.move_lines'] };
    expect(can('orders.cancel_pending', user)).toBe(true);
    expect(can('orders.cancel', user)).toBe(false);
  });

  test('sin usuario explícito lee la sesión viva', () => {
    useSessionStore.setState({ user: { ...cajero, permissions: ['orders.cancel'] } });
    expect(can('orders.cancel')).toBe(true);
    expect(can('payments.void')).toBe(false);
  });
});
