import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { useState } from 'react';

import { Provider } from '../../components/ui/provider';
import { useSessionStore } from '../../stores/session';
import type { NotificationView } from '../../api/notifications';
import { Campana } from './Campana';
import { FranjaDeAgotado, DURA_MS } from './FranjaDeAgotado';

const api = vi.hoisted(() => ({ list: vi.fn(), unread: vi.fn(), read: vi.fn(), bannerSeen: vi.fn(), resolve: vi.fn() }));
vi.mock('../../api/notifications', async (orig) => ({ ...(await orig<object>()), notificationsApi: api }));
class FakeES { addEventListener() {} close() {} }
vi.stubGlobal('EventSource', FakeES);

const aviso = (o: Partial<NotificationView>): NotificationView => ({
  id: 1, kind: 'stock_out', status: 'open', itemName: 'Bolsa grande', branchName: 'Matriz', onHandSeen: '0', minSeen: null,
  needsCount: false, openedAt: '2026-11-14T20:32:00Z', openedById: 7, openedByName: 'Ana', resolvedAt: null, read: false,
  banner: false, confirmations: 1, unitCode: 'pieza', ...o,
});

function montar(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><MemoryRouter>{ui}</MemoryRouter></Provider></QueryClientProvider>);
}

function ConCampana() {
  const [abierto, setAbierto] = useState(false);
  return <Campana abierto={abierto} setAbierto={setAbierto} />;
}

beforeEach(() => {
  useSessionStore.setState({ token: 't', status: 'authed', user: { id: 9, companyId: 1, name: 'Marisol', role: 'gerente',
    permissions: ['inventory.report_out', 'inventory.configure', 'inventory.receive', 'inventory.view'] } });
  api.unread.mockResolvedValue({ unread: 2 });
  api.read.mockResolvedValue(undefined);
  api.bannerSeen.mockResolvedValue(undefined);
  api.resolve.mockResolvedValue(undefined);
  api.list.mockImplementation((s: string) => Promise.resolve({ items: s === 'open'
    ? [aviso({ id: 1 }), aviso({ id: 2, kind: 'not_found', itemName: undefined, queryText: 'servitoallas', read: true })]
    : [aviso({ id: 3, status: 'resolved', resolvedReason: 'arrived', resolvedByName: 'Marisol', resolvedAt: '2026-11-14T21:00:00Z' })] }));
});
afterEach(() => { cleanup(); vi.clearAllMocks(); vi.useRealTimers(); document.body.innerHTML = ''; });

describe('Campana', () => {
  test('muestra lo no leído y al abrir marca como leído lo que ve, solo para esta persona', async () => {
    montar(<ConCampana />);
    const campana = await screen.findByRole('button', { name: 'Avisos, 2 sin leer' });
    fireEvent.click(campana);
    await waitFor(() => expect(api.read).toHaveBeenCalledWith([1]));
    expect(await screen.findByText('Bolsa grande')).toBeTruthy();
    expect(screen.getByText('«servitoallas» no está en el sistema')).toBeTruthy();
  });

  test('las cerradas dicen su motivo y quién', async () => {
    montar(<ConCampana />);
    fireEvent.click(await screen.findByRole('button', { name: /Avisos/ }));
    fireEvent.click(await screen.findByRole('tab', { name: 'Cerradas' }));
    expect(await screen.findByText(/Llegó · Marisol/)).toBeTruthy();
  });

  test('«Ya llegó» cierra el aviso', async () => {
    montar(<ConCampana />);
    fireEvent.click(await screen.findByRole('button', { name: /Avisos/ }));
    fireEvent.click(await screen.findByRole('button', { name: 'Ya llegó' }));
    await waitFor(() => expect(api.resolve).toHaveBeenCalledWith(1, 'arrived', undefined));
  });

  test('sin el permiso de reportar no hay campana', async () => {
    useSessionStore.setState({ user: { id: 9, companyId: 1, name: 'X', role: '', permissions: [] } });
    montar(<ConCampana />);
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByRole('button', { name: /Avisos/ })).toBeNull();
  });
});

describe('Franja de «se acabó»', () => {
  test('sale una vez, se guarda como vista y se quita sola a los 90 s', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    api.list.mockResolvedValue({ items: [aviso({ id: 5, banner: true })] });
    montar(<FranjaDeAgotado onVer={() => {}} />);
    expect(await screen.findByTestId('franja-de-agotado')).toBeTruthy();
    expect(api.bannerSeen).toHaveBeenCalledWith(5);
    await act(async () => { vi.advanceTimersByTime(DURA_MS + 100); });
    expect(screen.queryByTestId('franja-de-agotado')).toBeNull();
  });

  test('espera mientras haya una hoja abierta', async () => {
    api.list.mockResolvedValue({ items: [aviso({ id: 6, banner: true })] });
    const hoja = document.createElement('div');
    hoja.setAttribute('role', 'dialog');
    document.body.appendChild(hoja);
    montar(<FranjaDeAgotado onVer={() => {}} />);
    await new Promise((r) => setTimeout(r, 100));
    expect(screen.queryByTestId('franja-de-agotado')).toBeNull();
    expect(api.bannerSeen).not.toHaveBeenCalled();
    await act(async () => { hoja.remove(); });
    expect(await screen.findByTestId('franja-de-agotado')).toBeTruthy();
  });

  test('va debajo del aviso de pedido de plataforma, aunque crezca', async () => {
    api.list.mockResolvedValue({ items: [aviso({ id: 7, banner: true })] });
    const pedido = document.createElement('div');
    pedido.setAttribute('data-testid', 'aviso-de-pedido-entrante');
    pedido.getBoundingClientRect = () => ({ bottom: 92 } as DOMRect);
    document.body.appendChild(pedido);
    montar(<FranjaDeAgotado onVer={() => {}} />);
    const franja = await screen.findByTestId('franja-de-agotado');
    expect(getComputedStyle(franja).top).toBe('100px');
  });

  test('sin franja pedida en el artículo, nada', async () => {
    api.list.mockResolvedValue({ items: [aviso({ id: 8, banner: false })] });
    montar(<FranjaDeAgotado onVer={() => {}} />);
    await new Promise((r) => setTimeout(r, 100));
    expect(screen.queryByTestId('franja-de-agotado')).toBeNull();
  });
});
