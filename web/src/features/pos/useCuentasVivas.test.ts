import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider, focusManager } from '@tanstack/react-query';
import { createElement, type ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { useOrderEvents } from '../../hooks/useOrderEvents';
import { useSessionStore } from '../../stores/session';
import { useCuentasVivas } from './useCuentasVivas';

const api = vi.hoisted(() => ({ liveAccounts: vi.fn() }));
vi.mock('../../api/pos', () => ({ posApi: api }));

class FakeEventSource {
  static ultima: FakeEventSource | null = null;
  listeners: Record<string, Array<() => void>> = {};
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(public url: string) { FakeEventSource.ultima = this; }
  addEventListener(tipo: string, fn: () => void) { (this.listeners[tipo] ||= []).push(fn); }
  close() {}
  emitir(tipo: string) { (this.listeners[tipo] || []).forEach((f) => f()); }
}

let qc: QueryClient;
const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client: qc }, children);

beforeEach(() => {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.resetAllMocks();
  vi.stubGlobal('EventSource', FakeEventSource);
  useSessionStore.setState({ token: 'tok' });
  api.liveAccounts.mockResolvedValue({ items: [], outstanding: '0.00', serverTime: '' });
});
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });

describe('la fila de cuentas se mantiene al día (US1, US7)', () => {
  test('pide /pos/accounts sin las deudas viejas', async () => {
    renderHook(() => useCuentasVivas(), { wrapper });
    await waitFor(() => expect(api.liveAccounts).toHaveBeenCalledWith());
  });

  test('se vuelve a pedir cada 30 s', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderHook(() => useCuentasVivas(), { wrapper });
    await waitFor(() => expect(api.liveAccounts).toHaveBeenCalledTimes(1));
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });
    await waitFor(() => expect(api.liveAccounts).toHaveBeenCalledTimes(2));
  });

  test.each(['draft.updated', 'order.created', 'order.updated'])('el evento %s la refresca al instante', async (evento) => {
    renderHook(() => { useOrderEvents(); return useCuentasVivas(); }, { wrapper });
    await waitFor(() => expect(api.liveAccounts).toHaveBeenCalledTimes(1));
    act(() => FakeEventSource.ultima!.emitir(evento));
    await waitFor(() => expect(api.liveAccounts).toHaveBeenCalledTimes(2));
  });

  // El canal de eventos se cae en silencio en una tableta suspendida: al despertar la fila tiene
  // que ponerse al día sin que nadie toque nada.
  test('la tableta que vuelve de suspenderse refresca', async () => {
    renderHook(() => useCuentasVivas(), { wrapper });
    await waitFor(() => expect(api.liveAccounts).toHaveBeenCalledTimes(1));
    act(() => { focusManager.setFocused(false); });
    act(() => { focusManager.setFocused(true); });
    await waitFor(() => expect(api.liveAccounts).toHaveBeenCalledTimes(2));
    focusManager.setFocused(undefined);
  });
});
