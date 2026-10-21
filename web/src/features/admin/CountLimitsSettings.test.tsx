import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { useSessionStore } from '../../stores/session';
import { CountLimitsSettings } from './CountLimitsSettings';
import { DEFAULT_COUNT_LIMITS } from '../../api/countLimits';
import { ApiError } from '../../api/client';

const limits = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn() }));
vi.mock('../../api/countLimits', async (orig) => ({ ...(await orig<object>()), countLimitsApi: limits }));

const configura = () => useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'gerente', permissions: ['inventory.configure'] } as never });

beforeEach(() => {
  configura();
  limits.get.mockResolvedValue(structuredClone(DEFAULT_COUNT_LIMITS));
  limits.save.mockImplementation((l) => Promise.resolve(l));
});
afterEach(() => { cleanup(); document.body.innerHTML = ''; vi.clearAllMocks(); });

function montar() {
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <Provider><CountLimitsSettings /></Provider>
  </QueryClientProvider>);
}

describe('Límites del conteo (spec 044, US6)', () => {
  test('las cuatro clases en kg, L, m y piezas, con miles', async () => {
    montar();
    expect(await screen.findByLabelText('Máximo por conteo de Peso (kg)')).toHaveValue('1,000');
    expect(screen.getByLabelText('Pedir confirmar desde de Peso (kg)')).toHaveValue('50');
    expect(screen.getByLabelText('Máximo por conteo de Líquido (L)')).toHaveValue('1,000');
    expect(screen.getByLabelText('Máximo por conteo de Longitud (m)')).toHaveValue('10,000');
    expect(screen.getByLabelText('Pedir confirmar desde de Longitud (m)')).toHaveValue('50');
    expect(screen.getByLabelText('Máximo por conteo de Piezas (piezas)')).toHaveValue('1,000,000');
    expect(screen.getByLabelText('Pedir confirmar desde de Piezas (piezas)')).toHaveValue('5,000');
    expect(document.querySelector('select')).toBeNull();
  });

  test('«Guardar límites» manda el PUT en unidad base', async () => {
    montar();
    const peso = await screen.findByLabelText('Máximo por conteo de Peso (kg)');
    expect(screen.getByRole('button', { name: 'Guardar límites' })).toBeDisabled();
    fireEvent.focus(peso);
    fireEvent.change(peso, { target: { value: '2000' } });
    fireEvent.blur(peso);
    expect(peso).toHaveValue('2,000');
    fireEvent.click(screen.getByRole('button', { name: 'Guardar límites' }));
    await waitFor(() => expect(limits.save).toHaveBeenCalledWith({ ...DEFAULT_COUNT_LIMITS, mass: { max: 2_000_000, confirm: 50_000 } }));
  });

  test('máximo menor o igual que confirmar, cero o vacío: error en línea y no se guarda', async () => {
    montar();
    const peso = await screen.findByLabelText('Máximo por conteo de Peso (kg)');
    fireEvent.change(peso, { target: { value: '50' } });
    expect(screen.getByText('El máximo tiene que ser mayor que la cifra de confirmar.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Guardar límites' })).toBeDisabled();
    fireEvent.change(peso, { target: { value: '0' } });
    expect(screen.getByText('Escribe cifras mayores que cero.')).toBeInTheDocument();
    fireEvent.change(peso, { target: { value: 'abc' } });
    expect(screen.getByText('Escribe cifras mayores que cero.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Guardar límites' })).toBeDisabled();
  });

  test('«Descartar» vuelve a lo guardado', async () => {
    montar();
    const peso = await screen.findByLabelText('Máximo por conteo de Peso (kg)');
    fireEvent.change(peso, { target: { value: '7' } });
    fireEvent.click(screen.getByRole('button', { name: 'Descartar' }));
    expect(peso).toHaveValue('1,000');
    expect(limits.save).not.toHaveBeenCalled();
  });

  test('el rechazo del servidor se dice', async () => {
    limits.save.mockRejectedValue(new ApiError(400, 'VALIDATION', 'El máximo de peso pasa lo que el sistema guarda', 'r'));
    montar();
    const peso = await screen.findByLabelText('Máximo por conteo de Peso (kg)');
    fireEvent.change(peso, { target: { value: '9999999' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar límites' }));
    expect(await screen.findByText('El máximo de peso pasa lo que el sistema guarda')).toBeInTheDocument();
  });

  test('sin permiso de configurar el almacén no se ve', async () => {
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'cajero', permissions: ['inventory.count'] } as never });
    montar();
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByText('Límites del conteo')).toBeNull();
  });
  it('explica la sección sin jerga: sin «referencia»', async () => {
    montar();
    expect(await screen.findByText('Lo más que se puede registrar de un artículo en un conteo.')).toBeInTheDocument();
    expect(screen.getByText('Si lo contado pasa de esta cifra, el conteo pide confirmar.')).toBeInTheDocument();
    expect(screen.queryByText(/referencia/)).toBeNull();
  });
});
