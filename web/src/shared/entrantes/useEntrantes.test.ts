import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement, type ReactNode } from 'react';

import { useEntrantes, INTERVALO_DE_SONDEO } from './useEntrantes';
import * as api from '../../api/pedidosEntrantes';
import { useSessionStore } from '../../stores/session';

vi.mock('../../api/pedidosEntrantes', async () => {
  const real = await vi.importActual<typeof api>('../../api/pedidosEntrantes');
  return { ...real, pedidosEntrantes: vi.fn() };
});

// EventSource no existe en jsdom.
class FakeEventSource {
  static abiertas: FakeEventSource[] = [];
  listeners: Record<string, Array<() => void>> = {};
  constructor(public url: string) {
    FakeEventSource.abiertas.push(this);
  }
  addEventListener(tipo: string, fn: () => void) {
    (this.listeners[tipo] ||= []).push(fn);
  }
  close() {}
  emitir(tipo: string) {
    (this.listeners[tipo] || []).forEach((f) => f());
  }
}
const ultima = () => FakeEventSource.abiertas[FakeEventSource.abiertas.length - 1];

const pedido = { source: 'platform', id: 7, sourceName: 'Uber Eats', displayId: 'A1', customerName: 'Ana',
  customerPhone: '', address: '', serviceType: 'domicilio', total: '120.00', placedAt: null, decideBefore: null,
  lines: [] } as api.PedidoEntrante;

const conPermiso = (permisos: string[]) => useSessionStore.setState({
  token: 'tok', user: { id: 1, companyId: 2, name: 'Ana', role: 'cajero', permissions: permisos } as never,
});

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client: qc }, children);
  return renderHook(() => useEntrantes(), { wrapper });
}

describe('useEntrantes', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal('EventSource', FakeEventSource);
    FakeEventSource.abiertas = [];
    conPermiso(['orders.accept_incoming']);
    vi.mocked(api.pedidosEntrantes).mockResolvedValue([pedido]);
  });
  afterEach(() => vi.unstubAllGlobals());

  // RECARGA: la primera respuesta ya trae lo pendiente; no hace falta esperar un aviso.
  it('al montar trae lo pendiente', async () => {
    const { result } = montar();
    await waitFor(() => expect(result.current.pedidos).toHaveLength(1));
  });

  it('un aviso incoming.changed vuelve a pedir de inmediato', async () => {
    montar();
    await waitFor(() => expect(api.pedidosEntrantes).toHaveBeenCalledTimes(1));
    act(() => ultima().emitir('incoming.changed'));
    await waitFor(() => expect(api.pedidosEntrantes).toHaveBeenCalledTimes(2));
  });

  // RED CAÍDA Y RECONECTADA, o una subida que cambió de copia de la API: lo que pasó mientras no
  // había canal no llega como aviso, se recupera al reconectar.
  it('al reconectar el canal vuelve a pedir', async () => {
    montar();
    await waitFor(() => expect(api.pedidosEntrantes).toHaveBeenCalledTimes(1));
    act(() => {
      ultima().emitir('error');
      ultima().emitir('open');
    });
    await waitFor(() => expect(api.pedidosEntrantes).toHaveBeenCalledTimes(2));
  });

  // TABLETA SUSPENDIDA: al volver a primer plano pregunta, sin esperar al sondeo.
  it('al volver a primer plano vuelve a pedir', async () => {
    montar();
    await waitFor(() => expect(api.pedidosEntrantes).toHaveBeenCalledTimes(1));
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
    act(() => {
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await waitFor(() => expect(api.pedidosEntrantes).toHaveBeenCalledTimes(2));
  });

  // SIN RED, LA ALARMA SIGUE: un error de red no borra lo que ya se sabía pendiente (FR-021).
  it('un error de red conserva los pendientes conocidos', async () => {
    const { result } = montar();
    await waitFor(() => expect(result.current.pedidos).toHaveLength(1));
    vi.mocked(api.pedidosEntrantes).mockRejectedValue(new Error('sin red'));
    act(() => ultima().emitir('incoming.changed'));
    await waitFor(() => expect(api.pedidosEntrantes).toHaveBeenCalledTimes(2));
    expect(result.current.pedidos).toHaveLength(1);
  });

  // El sondeo es la red de abajo para un canal abierto pero muerto, y no más seguido: cada petición
  // queda en la bitácora.
  it('sondea a lo más cada 15 segundos', () => {
    expect(INTERVALO_DE_SONDEO).toBeLessThanOrEqual(15_000);
    expect(INTERVALO_DE_SONDEO).toBeGreaterThanOrEqual(10_000);
  });

  // SIN EL PERMISO, NADA: ni pregunta ni abre el canal.
  it('sin orders.accept_incoming no consulta ni escucha', async () => {
    conPermiso(['cash.operate']);
    const { result } = montar();
    await new Promise((r) => setTimeout(r, 20));
    expect(api.pedidosEntrantes).not.toHaveBeenCalled();
    expect(FakeEventSource.abiertas).toHaveLength(0);
    expect(result.current.pedidos).toEqual([]);
  });
});
