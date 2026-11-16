import { afterEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';

const changeOwnPassword = vi.hoisted(() => vi.fn());
const logout = vi.hoisted(() => vi.fn());
vi.mock('../api/pos', () => ({ posApi: { changeOwnPassword, logout } }));
vi.mock('../components/ui/toaster', () => ({ toaster: { create: vi.fn() } }));

import { RequireAuth } from './RequireAuth';
import { useSessionStore } from '../stores/session';

// EL CAMBIO OBLIGATORIO NO SE SALTA RECARGANDO (#311). El desvío vivía en la pantalla de login: una
// recarga entraba directo a la ruta pedida y se operaba con la contraseña que puso el admin. Ahora
// la decisión la toma el guardián de toda la aplicación con lo que dice la sesión, sin importar la
// ruta ni cómo se llegó a ella.

const usuario = { id: 7, companyId: 2, name: 'Ana', role: 'cajero', permissions: ['cash.operate'] };
const pintar = (ruta = '/pos') => render(
  <ChakraProvider value={defaultSystem}>
    <MemoryRouter initialEntries={[ruta]}>
      <RequireAuth><p>pantalla de vender</p></RequireAuth>
    </MemoryRouter>
  </ChakraProvider>,
);

afterEach(() => {
  useSessionStore.setState({ token: null, user: null, status: 'anon' });
  vi.clearAllMocks();
});

describe('cambio obligatorio de contraseña', () => {
  test('tras recargar en /pos con el indicador, se pide el cambio y no se pinta la aplicación', () => {
    useSessionStore.setState({ token: 't', status: 'authed', user: { ...usuario, mustChangePassword: true } });
    pintar('/pos');
    expect(screen.queryByText('pantalla de vender')).toBeNull();
    expect(screen.getByText('Debes cambiar tu contraseña')).toBeTruthy();
  });

  test('sin el indicador se pinta la aplicación', () => {
    useSessionStore.setState({ token: 't', status: 'authed', user: { ...usuario } });
    pintar('/pos');
    expect(screen.getByText('pantalla de vender')).toBeTruthy();
  });

  test('al cambiarla se apaga el indicador y aparece la aplicación', async () => {
    changeOwnPassword.mockResolvedValue(undefined);
    useSessionStore.setState({ token: 't', status: 'authed', user: { ...usuario, mustChangePassword: true } });
    pintar('/pos');
    fireEvent.change(screen.getByPlaceholderText('Contraseña actual'), { target: { value: 'Asignada-Por-El-Admin-1' } });
    fireEvent.change(screen.getByPlaceholderText('Nueva contraseña (mín. 12)'), { target: { value: 'Mi-Propia-Contrasena-9' } });
    fireEvent.change(screen.getByPlaceholderText('Repetir nueva contraseña'), { target: { value: 'Mi-Propia-Contrasena-9' } });
    fireEvent.click(screen.getByRole('button', { name: 'Cambiar contraseña' }));
    await waitFor(() => expect(screen.getByText('pantalla de vender')).toBeTruthy());
    expect(changeOwnPassword).toHaveBeenCalledWith('Asignada-Por-El-Admin-1', 'Mi-Propia-Contrasena-9');
    expect(useSessionStore.getState().user?.mustChangePassword).toBe(false);
  });

  test('se puede cerrar sesión sin cambiarla', async () => {
    logout.mockResolvedValue(undefined);
    useSessionStore.setState({ token: 't', status: 'authed', user: { ...usuario, mustChangePassword: true } });
    pintar('/pos');
    fireEvent.click(screen.getByRole('button', { name: 'Cerrar sesión' }));
    await waitFor(() => expect(useSessionStore.getState().status).toBe('anon'));
    expect(logout).toHaveBeenCalled();
  });
});
