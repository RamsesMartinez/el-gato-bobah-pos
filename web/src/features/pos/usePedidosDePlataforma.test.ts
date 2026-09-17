import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement, type ReactNode } from 'react';

import { usePedidosDePlataforma } from './usePedidosDePlataforma';
import * as api from '../../api/pedidosDePlataforma';
import { useSessionStore } from '../../stores/session';

vi.mock('../../api/pedidosDePlataforma', async () => {
  const real = await vi.importActual<typeof api>('../../api/pedidosDePlataforma');
  return { ...real, pedidosPendientes: vi.fn() };
});

// EventSource no existe en jsdom.
class FakeEventSource {
  static ultima: FakeEventSource | null = null;
  listeners: Record<string, Array<() => void>> = {};
  cerrada = false;
  constructor(public url: string) {
    FakeEventSource.ultima = this;
  }
  addEventListener(tipo: string, fn: () => void) {
    (this.listeners[tipo] ||= []).push(fn);
  }
  close() {
    this.cerrada = true;
  }
  emitir(tipo: string) {
    (this.listeners[tipo] || []).forEach((f) => f());
  }
}

const envolver = (qc: QueryClient) =>
  ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children);

describe('usePedidosDePlataforma', () => {
  beforeEach(() => {
    // El contador del mock se acumula entre casos y hace que el segundo empiece en 2.
    vi.clearAllMocks();
    vi.stubGlobal('EventSource', FakeEventSource);
    useSessionStore.setState({ token: 'tok' });
    vi.mocked(api.pedidosPendientes).mockResolvedValue([]);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    FakeEventSource.ultima = null;
  });

  // EL PEDIDO APARECE EN SEGUNDOS, que es el criterio de la feature: el evento en vivo dispara el
  // refresco sin esperar al siguiente sondeo.
  it('un pedido nuevo refresca la lista de inmediato', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    renderHook(() => usePedidosDePlataforma(), { wrapper: envolver(qc) });
    await waitFor(() => expect(api.pedidosPendientes).toHaveBeenCalledTimes(1));

    FakeEventSource.ultima!.emitir('platform.order.received');
    await waitFor(() => expect(api.pedidosPendientes).toHaveBeenCalledTimes(2));
  });

  // Y CUANDO OTRA TABLETA YA LO ATENDIÓ. Sin esto, la segunda sigue mostrando la alarma y quien la
  // toque encuentra un botón que ya no hace nada.
  it('se entera de que otra tableta ya decidió', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    renderHook(() => usePedidosDePlataforma(), { wrapper: envolver(qc) });
    await waitFor(() => expect(api.pedidosPendientes).toHaveBeenCalledTimes(1));

    FakeEventSource.ultima!.emitir('platform.order.decided');
    await waitFor(() => expect(api.pedidosPendientes).toHaveBeenCalledTimes(2));
  });

  // LA RED DE ABAJO. Si la conexión de eventos se cae, el evento se pierde y no hay forma de
  // recuperarlo: el pedido queda invisible hasta que la plataforma lo cancela y el cliente reclama.
  it('sondea aunque no llegue ningún evento', async () => {
    vi.useFakeTimers();
    try {
      const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
      renderHook(() => usePedidosDePlataforma(), { wrapper: envolver(qc) });
      await vi.advanceTimersByTimeAsync(0);
      expect(api.pedidosPendientes).toHaveBeenCalledTimes(1);

      // Pasa el intervalo SIN que llegue un solo evento: si la conexión se cayó, esto es lo único
      // que hace aparecer el pedido antes de que la plataforma lo cancele sola.
      await vi.advanceTimersByTimeAsync(31000);
      expect(vi.mocked(api.pedidosPendientes).mock.calls.length).toBeGreaterThan(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it('sin sesión no abre conexión de eventos', () => {
    useSessionStore.setState({ token: null });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    renderHook(() => usePedidosDePlataforma(), { wrapper: envolver(qc) });
    expect(FakeEventSource.ultima).toBeNull();
  });
});
