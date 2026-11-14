import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { useState } from 'react';

import { Provider } from '../../components/ui/provider';
import { useSessionStore } from '../../stores/session';
import type { NotificationView } from '../../api/notifications';
import { ApiError } from '../../api/client';
import { Campana } from './Campana';
import { FranjaDeAgotado, DURA_MS } from './FranjaDeAgotado';

const api = vi.hoisted(() => ({ list: vi.fn(), unread: vi.fn(), read: vi.fn(), bannerSeen: vi.fn(), resolve: vi.fn() }));
vi.mock('../../api/notifications', async (orig) => ({ ...(await orig<object>()), notificationsApi: api }));
const back = vi.hoisted(() => ({ articles: vi.fn() }));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
let abiertos = 0;
class FakeES { constructor() { abiertos++; } addEventListener() {} close() { abiertos--; } }
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

  // Campana y franja pedían cada una su conexión en vivo: dos por tableta y cada aviso refrescaba dos veces.
  test('la campana y la franja comparten una sola conexión en vivo', async () => {
    abiertos = 0;
    const { unmount } = montar(<><ConCampana /><FranjaDeAgotado onVer={() => {}} /></>);
    await screen.findByRole('button', { name: /Avisos/ });
    expect(abiertos).toBe(1);
    unmount();
    expect(abiertos).toBe(0);
  });

  // Con más de 50 sin leer la lista traía 50 y el contador nunca bajaba a 0.
  test('con 130 sin leer, abrir los marca todos, en tandas de 100', async () => {
    api.list.mockResolvedValue({ items: Array.from({ length: 130 }, (_, i) => aviso({ id: i + 1 })) });
    montar(<ConCampana />);
    await waitFor(() => expect(api.list).toHaveBeenCalled());
    await new Promise((r) => setTimeout(r, 20));
    fireEvent.click(await screen.findByRole('button', { name: /Avisos/ }));
    await waitFor(() => expect(api.read).toHaveBeenCalledTimes(2));
    expect(api.read.mock.calls[0][0]).toHaveLength(100);
    expect(api.read.mock.calls[1][0]).toHaveLength(30);
  });

  test('si cerrar un aviso falla porque ya lo cerró otro, la lista se vuelve a pedir', async () => {
    api.resolve.mockRejectedValueOnce(new ApiError(409, 'NOTIFICATION_CLOSED', 'ese aviso ya está cerrado', 'r'));
    montar(<ConCampana />);
    fireEvent.click(await screen.findByRole('button', { name: /Avisos/ }));
    const antes = api.list.mock.calls.length;
    fireEvent.click(await screen.findByRole('button', { name: 'Ya llegó' }));
    await screen.findByText('ese aviso ya está cerrado');
    await waitFor(() => expect(api.list.mock.calls.length).toBeGreaterThan(antes));
  });

  // Un toque errado en la lista ligaba para siempre un nombre al insumo equivocado.
  test('«Es otro nombre de…» pide confirmar antes de guardar el nombre', async () => {
    back.articles.mockResolvedValue([{ id: 40, name: 'Servilletas' }]);
    montar(<ConCampana />);
    fireEvent.click(await screen.findByRole('button', { name: /Avisos/ }));
    fireEvent.click(await screen.findByRole('button', { name: 'Es otro nombre de…' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Elegir insumo' }));
    fireEvent.click(await screen.findByText('Servilletas'));
    expect(api.resolve).not.toHaveBeenCalled();
    expect(await screen.findByText('Guardar «servitoallas» como otro nombre de «Servilletas»')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.resolve).toHaveBeenCalledWith(2, 'alias', 40));
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

  // Se marcaba vista y el reloj de 90 s seguía mientras una hoja la ocultaba: con un cobro largo,
  // nadie la veía.
  test('el reloj de 90 s no corre mientras una hoja la tapa', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    api.list.mockResolvedValue({ items: [aviso({ id: 9, banner: true })] });
    montar(<FranjaDeAgotado onVer={() => {}} />);
    await screen.findByTestId('franja-de-agotado');
    const hoja = document.createElement('div');
    hoja.setAttribute('role', 'dialog');
    await act(async () => { document.body.appendChild(hoja); });
    await waitFor(() => expect(screen.queryByTestId('franja-de-agotado')).toBeNull());
    await act(async () => { vi.advanceTimersByTime(DURA_MS + 1000); });
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

  // Cada mutación del cuerpo medía el aviso de pedido (un reflow): con el reloj de «Se acabó»
  // corriendo, eran decenas por segundo en la tableta.
  test('una ráfaga de cambios en la pantalla se mide una sola vez', async () => {
    api.list.mockResolvedValue({ items: [] });
    let medidas = 0;
    const pedido = document.createElement('div');
    pedido.setAttribute('data-testid', 'aviso-de-pedido-entrante');
    pedido.getBoundingClientRect = () => { medidas++; return { bottom: 0 } as DOMRect; };
    document.body.appendChild(pedido);
    montar(<FranjaDeAgotado onVer={() => {}} />);
    await new Promise((r) => setTimeout(r, 50));
    medidas = 0;
    await act(async () => {
      for (let i = 0; i < 30; i++) { document.body.appendChild(document.createElement('span')); await Promise.resolve(); }
      await new Promise((r) => setTimeout(r, 60));
    });
    expect(medidas).toBeLessThanOrEqual(2);
  });

  test('sin franja pedida en el artículo, nada', async () => {
    api.list.mockResolvedValue({ items: [aviso({ id: 8, banner: false })] });
    montar(<FranjaDeAgotado onVer={() => {}} />);
    await new Promise((r) => setTimeout(r, 100));
    expect(screen.queryByTestId('franja-de-agotado')).toBeNull();
  });
});
