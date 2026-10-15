import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';

import { Provider } from '../../components/ui/provider';
import { useSessionStore } from '../../stores/session';
import { PendingWasteBar } from './PendingWasteBar';

const waste = vi.hoisted(() => ({ pending: vi.fn(), approve: vi.fn(), reject: vi.fn() }));
vi.mock('../../api/waste', async (orig) => ({ ...(await orig<object>()), wasteApi: waste }));

const row = (id: number, name: string) => ({
  id, itemType: 'ingrediente', itemName: name, quantity: '3', unitCode: 'kg', totalCost: '285.00', status: 'pending',
  reason: 'Caducó', recordedBy: 'Ana', recordedAt: '2026-10-15T15:00:00Z', canUndo: false,
});

beforeEach(() => {
  try { sessionStorage.clear(); } catch { /* sin almacenamiento */ }
  waste.pending.mockResolvedValue([row(1, 'Perlas de tapioca'), row(2, 'Leche entera')]);
  waste.approve.mockResolvedValue(undefined);
  waste.reject.mockResolvedValue(undefined);
  useSessionStore.setState({ user: { id: 3, name: 'Gerente', role: 'gerente', permissions: ['inventory.write_off', 'inventory.write_off_approve'] } as never });
});
afterEach(() => {
  cleanup();
  document.body.innerHTML = '';
  vi.clearAllMocks();
});

function montar(path = '/almacen') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><MemoryRouter initialEntries={[path]}><PendingWasteBar /></MemoryRouter></Provider></QueryClientProvider>);
}

describe('mermas por aprobar (spec 039, maqueta B)', () => {
  test('una franja de 44 px dice cuántas esperan', async () => {
    montar();
    const bar = await screen.findByRole('button', { name: /2 esperan tu aprobación/ });
    expect(parseInt(getComputedStyle(bar).minHeight || '0', 10)).toBeGreaterThanOrEqual(44);
  });

  test('no sale en Vender ni en Pedidos: ahí el aviso es el número del riel', async () => {
    for (const path of ['/pos', '/pedidos']) {
      montar(path);
      await waitFor(() => expect(waste.pending).toHaveBeenCalled());
      expect(screen.queryByRole('button', { name: /esperan tu aprobación/ })).toBeNull();
      cleanup();
    }
  });

  test('sin el permiso de aprobar no pregunta ni pinta nada', async () => {
    useSessionStore.setState({ user: { id: 4, name: 'Ana', role: 'cajero', permissions: ['inventory.write_off'] } as never });
    montar();
    await new Promise((r) => setTimeout(r, 20));
    expect(waste.pending).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: /esperan/ })).toBeNull();
  });

  test('se cierra por la sesión', async () => {
    montar();
    await screen.findByRole('button', { name: /esperan tu aprobación/ });
    fireEvent.click(screen.getByRole('button', { name: 'Ocultar por ahora' }));
    expect(screen.queryByRole('button', { name: /esperan tu aprobación/ })).toBeNull();
    cleanup();
    montar();
    await waitFor(() => expect(waste.pending).toHaveBeenCalled());
    expect(screen.queryByRole('button', { name: /esperan tu aprobación/ })).toBeNull();
  });

  test('la hoja aprueba de un toque y rechaza con confirmación', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /esperan tu aprobación/ }));
    const perlas = (await screen.findByText(/Perlas de tapioca/)).closest('[data-testid="renglon-por-aprobar"]') as HTMLElement;
    fireEvent.click(within(perlas).getByRole('button', { name: 'Aprobar' }));
    await waitFor(() => expect(waste.approve).toHaveBeenCalledWith(1));
    const leche = screen.getByText(/Leche entera/).closest('[data-testid="renglon-por-aprobar"]') as HTMLElement;
    fireEvent.click(within(leche).getByRole('button', { name: 'Rechazar' }));
    expect(waste.reject).not.toHaveBeenCalled();
    fireEvent.click(within(leche).getByRole('button', { name: 'Sí, rechazar' }));
    await waitFor(() => expect(waste.reject).toHaveBeenCalledWith(2));
  });
});
