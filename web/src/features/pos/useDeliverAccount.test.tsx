import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, test, vi } from 'vitest';
import { ApiError } from '../../api/client';
import { cuenta } from './__fixtures__/cuentas';
import { useDeliverAccount } from './useDeliverAccount';

const api = vi.hoisted(() => ({ deliverOrder: vi.fn() }));
vi.mock('../../api/pos', () => ({ posApi: api }));
const toasts = vi.hoisted(() => ({ create: vi.fn() }));
vi.mock('../../components/ui/toaster', () => ({ toaster: toasts }));

let qc: QueryClient;
const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
const PAGADA = cuenta({ key: 'o:6', orderId: 6, folioName: 'Siamés', state: 'paid_in_kitchen', paid: '63.00', total: '63.00', outstanding: '0.00' });

beforeEach(() => {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.clearAllMocks();
});

// Entregar desde Vender (spec 089, research R-2): el mismo POST que «Entregar todo» en Pedidos.
describe('entregar una cuenta desde Vender', () => {
  test('entrega ESE pedido y relee la fila y los pedidos', async () => {
    api.deliverOrder.mockResolvedValue(undefined);
    const invalidar = vi.spyOn(qc, 'invalidateQueries');
    const { result } = renderHook(() => useDeliverAccount(), { wrapper });
    await act(async () => { result.current.deliver(PAGADA); });
    await waitFor(() => expect(api.deliverOrder).toHaveBeenCalledWith(6));
    await waitFor(() => expect(invalidar).toHaveBeenCalledWith({ queryKey: ['pos', 'accounts'] }));
    expect(invalidar).toHaveBeenCalledWith({ queryKey: ['orders'] });
    expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: 'Siamés: entregada', type: 'success' }));
  });

  // Revisión de la 089: con un candado único, un «Entregar» en OTRA cuenta mientras iba el primero se
  // perdía sin aviso y la cuenta se quedaba en la fila.
  test('entregar otra cuenta mientras va la primera también se manda', async () => {
    let soltar: () => void = () => {};
    api.deliverOrder.mockImplementation((id: number) => (id === 6 ? new Promise<void>((r) => { soltar = r; }) : Promise.resolve()));
    const otra = cuenta({ key: 'o:7', orderId: 7, folioName: 'Persa', state: 'paid_in_kitchen', outstanding: '0.00' });
    const { result } = renderHook(() => useDeliverAccount(), { wrapper });
    act(() => { result.current.deliver(PAGADA); });
    act(() => { result.current.deliver(otra); });
    await waitFor(() => expect(api.deliverOrder).toHaveBeenCalledWith(7));
    expect(api.deliverOrder).toHaveBeenCalledWith(6);
    await act(async () => { soltar(); });
    await waitFor(() => expect(result.current.delivering.size).toBe(0));
  });

  // Un dedo que toca dos veces en la hora pico no manda dos entregas (la segunda volvería con
  // conflicto y se leería como error).
  test('mientras entrega, un segundo toque no manda otra vez', async () => {
    let soltar: () => void = () => {};
    api.deliverOrder.mockReturnValue(new Promise<void>((r) => { soltar = r; }));
    const { result } = renderHook(() => useDeliverAccount(), { wrapper });
    act(() => { result.current.deliver(PAGADA); });
    await waitFor(() => expect(result.current.delivering.has('o:6')).toBe(true));
    act(() => { result.current.deliver(PAGADA); });
    expect(api.deliverOrder).toHaveBeenCalledTimes(1);
    await act(async () => { soltar(); });
    await waitFor(() => expect(result.current.delivering.size).toBe(0));
  });

  test('si el servidor la rechaza, lo dice en palabras de mostrador y relee la fila', async () => {
    api.deliverOrder.mockRejectedValue(new ApiError(409, 'conflict', 'este pedido ya está cerrado', 'req-1'));
    const invalidar = vi.spyOn(qc, 'invalidateQueries');
    const { result } = renderHook(() => useDeliverAccount(), { wrapper });
    await act(async () => { result.current.deliver(PAGADA); });
    await waitFor(() => expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: 'No se pudo entregar', type: 'error' })));
    const desc = toasts.create.mock.calls[0][0].description as string;
    expect(desc).not.toMatch(/Error:|409|conflict|TypeError/);
    expect(invalidar).toHaveBeenCalledWith({ queryKey: ['pos', 'accounts'] });
  });

  test('una cuenta en captura no se manda a entregar: todavía no es pedido', async () => {
    const { result } = renderHook(() => useDeliverAccount(), { wrapper });
    act(() => { result.current.deliver(cuenta({ key: 'd:a', kind: 'draft', orderId: null, state: 'capturing' })); });
    expect(api.deliverOrder).not.toHaveBeenCalled();
  });
});
