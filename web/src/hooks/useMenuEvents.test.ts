import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement, type ReactNode } from 'react';

import { useMenuEvents } from './useMenuEvents';
import { useSessionStore } from '../stores/session';

// EventSource no existe en jsdom. El doble guarda los listeners para poder disparar el evento a
// mano, que es justo lo que se quiere probar: que `menu.updated` haga algo.
class FakeEventSource {
  static ultima: FakeEventSource | null = null;
  listeners: Record<string, Array<() => void>> = {};
  cerrada = false;
  constructor(public url: string) {
    FakeEventSource.ultima = this;
  }
  addEventListener(tipo: string, fn: () => void) {
    (this.listeners[tipo] ??= []).push(fn);
  }
  emitir(tipo: string) {
    (this.listeners[tipo] ?? []).forEach((f) => f());
  }
  close() {
    this.cerrada = true;
  }
}

function envoltura(qc: QueryClient) {
  return ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children);
}

describe('useMenuEvents', () => {
  beforeEach(() => {
    vi.stubGlobal('EventSource', FakeEventSource);
    useSessionStore.setState({ token: 'tok' });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    FakeEventSource.ultima = null;
  });

  it('al llegar menu.updated vuelve a pedir el menú', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidar = vi.spyOn(qc, 'invalidateQueries');
    renderHook(() => useMenuEvents(), { wrapper: envoltura(qc) });

    FakeEventSource.ultima!.emitir('menu.updated');

    await waitFor(() =>
      expect(invalidar).toHaveBeenCalledWith(expect.objectContaining({ queryKey: ['menu'] })),
    );
  });

  // Sin sesión no hay a quién suscribirse, y abrir el EventSource igual dejaría al backend
  // rechazando una conexión por tablet en la pantalla de login.
  it('sin token no abre la conexión', () => {
    useSessionStore.setState({ token: null });
    const qc = new QueryClient();
    renderHook(() => useMenuEvents(), { wrapper: envoltura(qc) });
    expect(FakeEventSource.ultima).toBeNull();
  });

  it('al desmontar cierra la conexión', () => {
    const qc = new QueryClient();
    const { unmount } = renderHook(() => useMenuEvents(), { wrapper: envoltura(qc) });
    const es = FakeEventSource.ultima!;
    unmount();
    expect(es.cerrada).toBe(true);
  });
});
