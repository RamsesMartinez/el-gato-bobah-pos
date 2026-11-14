import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';

import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import { SeAcaboPage } from './SeAcaboPage';

const out = vi.hoisted(() => ({ search: vi.fn(), suggestions: vi.fn(), report: vi.fn(), undo: vi.fn(), notFound: vi.fn() }));
vi.mock('../../api/stockOut', async (orig) => ({ ...(await orig<object>()), stockOutApi: out }));

const queso = { itemType: 'ingrediente' as const, id: 1, name: 'Queso crema', unit: 'g' };
const bolsa = { itemType: 'ingrediente' as const, id: 2, name: 'Bolsa grande', unit: 'pieza' };

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <MemoryRouter><SeAcaboPage /></MemoryRouter>
      </Provider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  out.suggestions.mockResolvedValue({ frequent: [queso, bolsa], today: [], undoSeconds: 90 });
  out.search.mockResolvedValue({ items: [queso] });
  out.report.mockResolvedValue({ id: 7, kind: 'report', notificationId: 3, delta: '-3', undoUntil: new Date(Date.now() + 90_000).toISOString(), confirmations: 1 });
  out.undo.mockResolvedValue(undefined);
  out.notFound.mockResolvedValue({ id: 8, notificationId: 4 });
});
afterEach(() => { cleanup(); vi.clearAllMocks(); vi.useRealTimers(); });

describe('Se acabó', () => {
  test('el primer toque arma el mosaico y el segundo reporta; nunca manda cantidad', async () => {
    montar();
    const mosaico = await screen.findByRole('button', { name: /Queso crema/ });
    fireEvent.click(mosaico);
    expect(out.report).not.toHaveBeenCalled();
    expect(screen.getByText('Toca otra vez para avisar que se acabó')).toBeTruthy();
    await new Promise((r) => setTimeout(r, 450));
    fireEvent.click(screen.getByRole('button', { name: /Queso crema/ }));
    await waitFor(() => expect(out.report).toHaveBeenCalledTimes(1));
    const [item, key] = out.report.mock.calls[0];
    expect(item).toEqual(queso);
    expect(key).toMatch(/^[0-9a-f-]{36}$/);
    expect(JSON.stringify(out.report.mock.calls[0])).not.toMatch(/quantity|qty/);
    expect(await screen.findByText(/Queso crema quedó en 0/)).toBeTruthy();
    expect(screen.getByRole('button', { name: /Deshacer/ })).toBeTruthy();
  });

  test('un doble toque rápido no confirma (0.4 s de gracia)', async () => {
    montar();
    const mosaico = await screen.findByRole('button', { name: /Queso crema/ });
    fireEvent.click(mosaico);
    fireEvent.click(mosaico);
    await new Promise((r) => setTimeout(r, 50));
    expect(out.report).not.toHaveBeenCalled();
  });

  test('tocar otro mueve el armado y no reporta dos', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Queso crema/ }));
    await new Promise((r) => setTimeout(r, 450));
    fireEvent.click(screen.getByRole('button', { name: /Bolsa grande/ }));
    expect(out.report).not.toHaveBeenCalled();
    expect(screen.getAllByText('Toca otra vez para avisar que se acabó')).toHaveLength(1);
  });

  test('el armado se quita solo a los 4 s', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Queso crema/ }));
    expect(screen.queryByText('Toca otra vez para avisar que se acabó')).toBeTruthy();
    await act(async () => { vi.advanceTimersByTime(4100); });
    expect(screen.queryByText('Toca otra vez para avisar que se acabó')).toBeNull();
  });

  test('escribir quita el armado y busca', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Queso crema/ }));
    fireEvent.change(screen.getByRole('textbox', { name: '¿Qué se acabó?' }), { target: { value: 'quesos' } });
    expect(screen.queryByText('Toca otra vez para avisar que se acabó')).toBeNull();
    await waitFor(() => expect(out.search).toHaveBeenCalledWith('quesos'));
  });

  test('«No lo encuentro» también pide un segundo toque y no toca existencias', async () => {
    out.search.mockResolvedValue({ items: [] });
    montar();
    fireEvent.change(await screen.findByRole('textbox', { name: '¿Qué se acabó?' }), { target: { value: 'servitoallas' } });
    const fila = await screen.findByRole('button', { name: /No lo encuentro/ });
    fireEvent.click(fila);
    expect(out.notFound).not.toHaveBeenCalled();
    await new Promise((r) => setTimeout(r, 450));
    fireEvent.click(screen.getByRole('button', { name: /No lo encuentro/ }));
    await waitFor(() => expect(out.notFound).toHaveBeenCalledWith('servitoallas', expect.stringMatching(/^[0-9a-f-]{36}$/)));
    expect(out.report).not.toHaveBeenCalled();
    expect(await screen.findByText(/Avisamos «servitoallas»/)).toBeTruthy();
  });

  test('el tope por hora se dice en pantalla', async () => {
    out.report.mockRejectedValue(new ApiError(429, 'REPORT_LIMIT', 'llegaste al límite de 20 reportes por hora; pide a quien compra que revise los avisos', 'r'));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Queso crema/ }));
    await new Promise((r) => setTimeout(r, 450));
    fireEvent.click(screen.getByRole('button', { name: /Queso crema/ }));
    expect(await screen.findByText(/límite de 20 reportes por hora/)).toBeTruthy();
  });

  test('el aviso de deshacer vive en un hueco de alto fijo: los mosaicos no se mueven', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Queso crema/ }));
    expect(screen.getByTestId('hueco-del-aviso').getAttribute('data-alto')).toBe('76');
    await new Promise((r) => setTimeout(r, 450));
    fireEvent.click(screen.getByRole('button', { name: /Queso crema/ }));
    await screen.findByText(/Queso crema quedó en 0/);
    expect(screen.getByTestId('hueco-del-aviso').getAttribute('data-alto')).toBe('76');
  });
});
