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

  // El servidor normaliza el correo; la pantalla comparaba contra lo que se escribió y «Guardar» se
  // quedaba encendido para siempre, con «Guardado.» a la vista aunque hubiera cambios nuevos.
  test('tras guardar toma lo que el servidor dejó y «Guardado.» se va al cambiar algo', async () => {
    montar();
    fireEvent.change(await screen.findByLabelText('Correo extra'), { target: { value: 'Compras@Ejemplo.MX' } });
    fireEvent.click(screen.getByRole('button', { name: 'Agregar' }));
    api.get.mockResolvedValue({ morningAt: '08:00', extras: [{ email: 'compras@ejemplo.mx', stock: true, notFound: true }], people: [
      { userId: 1, name: 'Marisol', role: 'Gerente', email: 'marisol@ejemplo.mx', hasEmail: true, stock: true, notFound: true },
      { userId: 2, name: 'Ramiro', role: 'Gerente', email: '', hasEmail: false, stock: true, notFound: true },
    ] });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    expect(await screen.findByText('Guardado.')).toBeTruthy();
    await waitFor(() => expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled());
    expect(screen.getByText('compras@ejemplo.mx')).toBeTruthy();
    await userEvent.click(screen.getByRole('checkbox', { name: 'Marisol recibe «no lo encuentro»' }));
    expect(screen.queryByText('Guardado.')).toBeNull();
  });

  test('cada interruptor se toca en 44 px', async () => {
    montar();
    const raiz = (await screen.findByRole('checkbox', { name: 'Marisol recibe «se acabó»' })).closest('label')!;
    expect(getComputedStyle(raiz).minHeight).toBe('44px');
    expect(getComputedStyle(raiz).minWidth).toBe('44px');
  });

  // Con varias personas y correos, «Guardar» quedaba al fondo de lo que se desplaza, fuera de los 854 px.
  test('«Guardar» vive fuera de lo que se desplaza', async () => {
    montar();
    const guardar = await screen.findByRole('button', { name: 'Guardar' });
    const lista = screen.getByTestId('destinatarios-lista');
    expect(getComputedStyle(lista).overflowY).toBe('auto');
    expect(lista.contains(guardar)).toBe(false);
  });
});
