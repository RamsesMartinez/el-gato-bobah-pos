import { vi } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { createElement } from 'react';

const addOrderLines = vi.hoisted(() => vi.fn());
vi.mock('../../api/pos', () => ({
  posApi: { addOrderLines, createOrder: vi.fn(), businessSettings: vi.fn(async () => ({ deliveryFee: '20' })) },
}));
// El menú solo trae el producto 1: el 2 lo desactivó alguien mientras estaba en el carrito.
vi.mock('../../hooks/useMenu', () => ({
  useMenu: () => ({ data: { products: [{ id: 1, price: '50' }] } }),
}));
vi.mock('../../components/ui/toaster', () => ({ toaster: { create: vi.fn() } }));

import { useAgregarAPedido } from './useAgregarAPedido';
import { useTicketStore } from '../../stores/ticket';
import type { BoardOrder } from '../../types/pos';

function envoltura({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client: qc }, children);
}

// UN PRODUCTO MUERTO NO PUEDE TIRAR EL CARRITO ENTERO.
//
// Confirmar excluye los renglones cuyo producto se inactivó mientras estaba en el carrito, "para no
// tumbar el pedido entero por un renglón". Agregar a un pedido en curso nació sin ese recorte y
// mandaba `cuenta.lines` completo: el servidor rechazaba con 422 y NO entraba nada, mientras el
// mismo carrito por el botón de confirmar sí pasaba. El hermano que no se movió.
test('agregar excluye los productos que ya no están en el menú', async () => {
  addOrderLines.mockResolvedValue({ id: 12, agregados: [] });
  useTicketStore.setState(useTicketStore.getInitialState(), true);
  const s = useTicketStore.getState();
  s.addLine({ productId: 1, name: 'Vivo', unitPrice: 50, qty: 1, modifiers: [] });
  s.addLine({ productId: 2, name: 'Desactivado', unitPrice: 80, qty: 1, modifiers: [] });

  const { result } = renderHook(() => useAgregarAPedido(() => {}), { wrapper: envoltura });
  act(() => result.current.agregar({ id: 12 } as BoardOrder));

  await waitFor(() => expect(addOrderLines).toHaveBeenCalled());
  const [, lineas] = addOrderLines.mock.calls[0];
  expect(lineas.map((l: { productId: number }) => l.productId)).toEqual([1]);
});
