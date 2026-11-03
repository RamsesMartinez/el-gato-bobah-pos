import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement, type ReactNode } from 'react';

import { useOrderEvents } from './useOrderEvents';
import { useSessionStore } from '../stores/session';

// EventSource no existe en jsdom.
class FakeEventSource {
  static ultima: FakeEventSource | null = null;
  listeners: Record<string, Array<() => void>> = {};
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(public url: string) {
    FakeEventSource.ultima = this;
  }
  addEventListener(tipo: string, fn: () => void) {
    (this.listeners[tipo] ??= []).push(fn);
  }
  emitir(tipo: string) {
    (this.listeners[tipo] ?? []).forEach((f) => f());
    if (tipo === 'open') this.onopen?.();
    if (tipo === 'error') this.onerror?.();
  }
  close() {}
}

const envoltura = (qc: QueryClient) =>
  ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children);

describe('useOrderEvents', () => {
  beforeEach(() => {
    vi.stubGlobal('EventSource', FakeEventSource);
    useSessionStore.setState({ token: 'tok' });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    FakeEventSource.ultima = null;
  });

  // Durante una subida (spec 041) un pedido creado en la copia vieja de la API no le llega como
  // aviso a la tableta que ya reconectó a la nueva; el tablero tiene que volver a pedir.
  it('al reconectarse vuelve a pedir los pedidos y las cuentas, y al conectarse la primera vez no', async () => {
    const qc = new QueryClient();
    const invalidar = vi.spyOn(qc, 'invalidateQueries');
    renderHook(() => useOrderEvents(), { wrapper: envoltura(qc) });
    const es = FakeEventSource.ultima!;

    es.emitir('open');
    expect(invalidar).not.toHaveBeenCalled();

    es.emitir('error');
    es.emitir('open');
    await waitFor(() => {
      expect(invalidar).toHaveBeenCalledWith({ queryKey: ['orders'] });
      expect(invalidar).toHaveBeenCalledWith({ queryKey: ['pos', 'accounts'] });
    });
  });

  it('un pedido nuevo invalida los pedidos', () => {
    const qc = new QueryClient();
    const invalidar = vi.spyOn(qc, 'invalidateQueries');
    renderHook(() => useOrderEvents(), { wrapper: envoltura(qc) });
    FakeEventSource.ultima!.emitir('order.created');
    expect(invalidar).toHaveBeenCalledWith({ queryKey: ['orders'] });
  });
});
