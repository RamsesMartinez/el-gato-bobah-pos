import { vi } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { createElement } from 'react';

const createOrder = vi.hoisted(() => vi.fn());
vi.mock('../../api/pos', () => ({
  posApi: {
    createOrder,
    addOrderLines: vi.fn(),
    businessSettings: vi.fn(async () => ({ deliveryFee: '20' })),
  },
}));
vi.mock('../../hooks/useMenu', () => ({ useMenu: () => ({ data: { products: [] } }) }));

import { useMandarPedido } from './useMandarPedido';
import { useTicketStore } from '../../stores/ticket';

function envoltura({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client: qc }, children);
}

// UN REINTENTO NO PUEDE CREAR DOS PEDIDOS CON LO MISMO.
//
// El servidor tiene idempotencia por `client_uuid` —la columna es única y devuelve el pedido que ya
// existe— pero el hook generaba un uuid NUEVO dentro de cada intento, así que esa protección nunca
// se disparaba: un corte de red al confirmar, un reintento, y quedaban dos pedidos idénticos. El
// operador cobra uno y el otro se queda abierto pidiendo comida que nadie preparó.
//
// El identificador tiene que ser el de la CUENTA: se genera al abrirla, se persiste, y sobrevive al
// reintento y a la recarga.
test('el reintento manda el MISMO identificador de pedido', async () => {
  createOrder.mockRejectedValueOnce(new Error('red caída'));
  createOrder.mockResolvedValueOnce({ id: 1, number: 1, lines: [] });

  // Se lee ANTES: al confirmar con éxito la cuenta se cierra y la activa pasa a ser otra.
  const cuenta = useTicketStore.getState().activeId;
  const { result } = renderHook(() => useMandarPedido(() => {}), { wrapper: envoltura });

  act(() => { result.current.mandar({}); });
  await waitFor(() => expect(createOrder).toHaveBeenCalledTimes(1));
  act(() => { result.current.mandar({}); });
  await waitFor(() => expect(createOrder).toHaveBeenCalledTimes(2));

  const primero = createOrder.mock.calls[0][0].clientUuid;
  const reintento = createOrder.mock.calls[1][0].clientUuid;
  expect(reintento).toBe(primero);
  // Y es el de la cuenta, no uno inventado por el hook.
  expect(primero).toBe(cuenta);
});
