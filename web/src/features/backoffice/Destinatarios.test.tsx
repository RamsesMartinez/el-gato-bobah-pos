import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import userEvent from '@testing-library/user-event';

import { Provider } from '../../components/ui/provider';
import { Destinatarios } from './Destinatarios';

const api = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn() }));
vi.mock('../../api/notifications', async (orig) => ({ ...(await orig<object>()), recipientsApi: api }));

const montar = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><Destinatarios /></Provider></QueryClientProvider>);
};

beforeEach(() => {
  api.get.mockResolvedValue({ morningAt: '08:00', extras: [], people: [
    { userId: 1, name: 'Marisol', role: 'Gerente', email: 'marisol@ejemplo.mx', hasEmail: true, stock: true, notFound: true },
    { userId: 2, name: 'Ramiro', role: 'Gerente', email: '', hasEmail: false, stock: true, notFound: true },
  ] });
  api.save.mockResolvedValue(undefined);
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

describe('Destinatarios', () => {
  test('quien no tiene correo lo dice y no se puede encender', async () => {
    montar();
    expect(await screen.findByText('Sin correo: captúralo en Empleados para que reciba')).toBeTruthy();
    expect(screen.getByRole('checkbox', { name: 'Ramiro recibe «se acabó»' })).toBeDisabled();
  });

  test('quitar a uno y agregar un correo extra se guardan juntos', async () => {
    montar();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Marisol recibe «no lo encuentro»' }));
    fireEvent.change(screen.getByLabelText('Correo extra'), { target: { value: 'compras@ejemplo.mx' } });
    fireEvent.click(screen.getByRole('button', { name: 'Agregar' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.save).toHaveBeenCalledWith(expect.objectContaining({
      people: expect.arrayContaining([expect.objectContaining({ userId: 1, stock: true, notFound: false })]),
      extras: [{ email: 'compras@ejemplo.mx', stock: true, notFound: true }],
    })));
  });

  test('sin cambios no se guarda', async () => {
    montar();
    await screen.findByText('Marisol');
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
  });
});
