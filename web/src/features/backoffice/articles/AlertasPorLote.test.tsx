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
  // La ficha captura el mínimo en kg y el lote lo pedía en g: quien escribía «2» pensando en kilos
  // programaba un aviso a 2 g.
  test('solo manda lo que cambió, con el mínimo en kg como la ficha y la confirmación antes', async () => {
    const onClose = montar();
    expect(await screen.findByText('kg')).toBeTruthy();
    fireEvent.change(await screen.findByLabelText('Mínimo de Queso crema'), { target: { value: '2' } });
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

  // Encender la alerta mandaba también el mínimo leído al abrir la tabla: si otra persona lo cambió en la
  // ficha mientras tanto, se regresaba.
  test('tocar solo el interruptor no manda el mínimo', async () => {
    montar();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Alerta de Vaso 16 oz' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios (1)' }));
    await waitFor(() => expect(api.batch).toHaveBeenCalledWith([{ itemType: 'ingrediente', id: 2, enabled: false }], true));
  });

  test('un mínimo que no es número se marca en su renglón y no sale', async () => {
    montar();
    fireEvent.change(await screen.findByLabelText('Mínimo de Queso crema'), { target: { value: 'abc' } });
    fireEvent.change(screen.getByLabelText('Mínimo de Vaso 16 oz'), { target: { value: '-5' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios (2)' }));
    expect(await screen.findByText(/Revisa el mínimo de Queso crema y Vaso 16 oz/)).toBeTruthy();
    expect(api.batch).not.toHaveBeenCalled();
    expect(screen.getByLabelText('Mínimo de Queso crema').getAttribute('aria-invalid')).toBe('true');
  });

  test('las comas valen como punto y el 0 es «sin mínimo»', async () => {
    montar();
    fireEvent.change(await screen.findByLabelText('Mínimo de Queso crema'), { target: { value: '1,5' } });
    fireEvent.change(screen.getByLabelText('Mínimo de Vaso 16 oz'), { target: { value: '0' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios (2)' }));
    await waitFor(() => expect(api.batch).toHaveBeenCalledWith([
      { itemType: 'ingrediente', id: 1, enabled: false, minStock: '1500' },
      { itemType: 'ingrediente', id: 2, enabled: true, minStock: null },
    ], true));
  });

  test('el interruptor de cada renglón se toca en 44 px', async () => {
    montar();
    const raiz = (await screen.findByRole('checkbox', { name: 'Alerta de Queso crema' })).closest('label')!;
    expect(getComputedStyle(raiz).minHeight).toBe('44px');
    expect(getComputedStyle(raiz).minWidth).toBe('44px');
  });
});
