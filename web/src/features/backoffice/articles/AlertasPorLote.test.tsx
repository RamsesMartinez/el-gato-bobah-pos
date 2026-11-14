import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import userEvent from '@testing-library/user-event';

import { Provider } from '../../../components/ui/provider';
import { AlertasPorLote } from './AlertasPorLote';

const api = vi.hoisted(() => ({ get: vi.fn(), preview: vi.fn(), save: vi.fn(), asked: vi.fn(), list: vi.fn(), batch: vi.fn() }));
vi.mock('../../../api/notifications', async (orig) => ({ ...(await orig<object>()), lowStockApi: api }));

const montar = (onClose = vi.fn()) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><AlertasPorLote onClose={onClose} /></Provider></QueryClientProvider>);
  return onClose;
};

beforeEach(() => {
  api.list.mockResolvedValue({ items: [
    { itemType: 'ingrediente', id: 1, name: 'Queso crema', unitCode: 'g', enabled: false, minStock: null, onHand: '1200' },
    { itemType: 'ingrediente', id: 2, name: 'Vaso 16 oz', unitCode: 'pieza', enabled: true, minStock: '200', onHand: '180' },
  ] });
  api.batch.mockImplementation((_items: unknown, preview: boolean) => Promise.resolve({ items: preview
    ? [{ itemType: 'ingrediente', id: 1, effect: { opens: ['stock_low'], closes: [] }, onHand: '1200' }] : [] }));
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

describe('Alertas por lote', () => {
  test('solo manda lo que cambió, con el mínimo en la unidad base y la confirmación antes', async () => {
    const onClose = montar();
    fireEvent.change(await screen.findByLabelText('Mínimo de Queso crema'), { target: { value: '2000' } });
    await userEvent.click(screen.getByRole('checkbox', { name: 'Alerta de Queso crema' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios (1)' }));
    await waitFor(() => expect(api.batch).toHaveBeenCalledWith([{ itemType: 'ingrediente', id: 1, enabled: true, minStock: '2000' }], true));
    expect(await screen.findByText(/Queso crema \(quedan [^)]+\) ya está en o bajo su mínimo/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.batch).toHaveBeenLastCalledWith(expect.any(Array), false));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  test('borrar el mínimo es «sin mínimo», no 0', async () => {
    montar();
    fireEvent.change(await screen.findByLabelText('Mínimo de Vaso 16 oz'), { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios (1)' }));
    await waitFor(() => expect(api.batch).toHaveBeenCalledWith([{ itemType: 'ingrediente', id: 2, enabled: true, minStock: null }], true));
  });

  test('la búsqueda filtra sin acentos', async () => {
    montar();
    fireEvent.change(await screen.findByLabelText('Buscar insumo'), { target: { value: 'QUESO' } });
    expect(screen.queryByText('Vaso 16 oz')).toBeNull();
    expect(screen.getByText('Queso crema')).toBeTruthy();
  });
});
