import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, test, vi } from 'vitest';
import { ApiError } from '../../api/client';
import { usePosStore } from '../../stores/pos';
import type { DraftView, OrderView } from '../../types/pos';
import { useEnviarCuenta } from './useEnviarCuenta';

const api = vi.hoisted(() => ({ sendDraft: vi.fn(), createDraft: vi.fn(), discardDraft: vi.fn(), getDraft: vi.fn() }));
vi.mock('../../api/pos', () => ({ posApi: api }));
const toasts = vi.hoisted(() => ({ create: vi.fn() }));
vi.mock('../../components/ui/toaster', () => ({ toaster: toasts }));

const PEDIDO = { id: 12, number: 3, folioName: 'Levkoy', total: '74.00', outstanding: '74.00' } as OrderView;
const CUENTA: DraftView = {
  id: 'd-1', orderId: 12, folioName: null, status: 'capturando', headerVersion: 1, version: 4, updatedAt: '', createdAt: '',
  openedBy: 'Ana', serviceType: 'mostrador', customerName: null, platformId: null, platformOrderRef: null,
  deliveryFee: '0.00', discount: null, subtotal: '29.00', discountTotal: '0.00', total: '29.00', unavailable: [],
  lines: [{ id: 'l-1', version: 1, productId: 41, productName: 'Coca', qty: '2', unitPrice: '14.50',
    modifiers: [{ optionId: 7, name: 'Hielo', qty: 1, priceDelta: '0.00' }], notes: 'fría', lineTotal: '29.00', available: true }],
};

let qc: QueryClient;
const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;

beforeEach(() => {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.clearAllMocks();
  usePosStore.setState({ selected: { kind: 'draft', id: 'd-1' } });
});

describe('mandar a cocina (US3, caso 5)', () => {
  test('imprime la comanda con los renglones que dice el servidor', async () => {
    api.sendDraft.mockResolvedValue({ order: PEDIDO, printLineIds: [501, 502], created: true });
    const onComanda = vi.fn();
    const { result } = renderHook(() => useEnviarCuenta({ onComanda }), { wrapper });
    await act(async () => { await result.current.enviar('d-1'); });
    expect(onComanda).toHaveBeenCalledWith(PEDIDO, [501, 502]);
  });

  // Un reintento que el servidor reconoce regresa sin renglones: reimprimir mandaría la comanda dos
  // veces a cocina.
  test('el reintento con printLineIds vacío no reimprime', async () => {
    api.sendDraft.mockResolvedValue({ order: PEDIDO, printLineIds: [], created: false });
    const onComanda = vi.fn();
    const { result } = renderHook(() => useEnviarCuenta({ onComanda }), { wrapper });
    await act(async () => { await result.current.enviar('d-1'); });
    expect(onComanda).not.toHaveBeenCalled();
  });

  // Caso 5: mandar a cocina borraba la cuenta de la pantalla y nada recordaba que faltaba cobrar.
  test('la cuenta enviada se queda seleccionada, ahora como pedido', async () => {
    api.sendDraft.mockResolvedValue({ order: PEDIDO, printLineIds: [1], created: true });
    const { result } = renderHook(() => useEnviarCuenta({ onComanda: vi.fn() }), { wrapper });
    await act(async () => { await result.current.enviar('d-1'); });
    expect(usePosStore.getState().selected).toEqual({ kind: 'order', id: 12 });
    expect(qc.getQueryData(['orders', 12])).toEqual(PEDIDO);
  });

  test('devuelve el pedido para que cobrar siga con él', async () => {
    api.sendDraft.mockResolvedValue({ order: PEDIDO, printLineIds: [1], created: true });
    const { result } = renderHook(() => useEnviarCuenta({ onComanda: vi.fn() }), { wrapper });
    let r: OrderView | null = null;
    await act(async () => { r = await result.current.enviar('d-1'); });
    expect(r).toEqual(PEDIDO);
  });

  test('si falla, devuelve null y deja el motivo para el pie', async () => {
    api.sendDraft.mockRejectedValue(new ApiError(409, 'NO_OPEN_REGISTER', 'No hay caja abierta', 'r'));
    const { result } = renderHook(() => useEnviarCuenta({ onComanda: vi.fn() }), { wrapper });
    let r: OrderView | null = PEDIDO;
    await act(async () => { r = await result.current.enviar('d-1'); });
    expect(r).toBeNull();
    expect(result.current.motivo).toBe('No hay caja abierta');
    expect(usePosStore.getState().selected).toEqual({ kind: 'draft', id: 'd-1' });
  });
});

describe('a lo «Nuevo» de un pedido que ya se cerró (research R-9)', () => {
  test('ofrece empezar una cuenta nueva con los mismos productos y descarta la vieja', async () => {
    qc.setQueryData(['pos', 'draft', 'd-1'], CUENTA);
    api.sendDraft.mockRejectedValue(new ApiError(409, 'ORDER_CLOSED', 'x', 'r'));
    api.createDraft.mockImplementation(async (b) => ({ ...CUENTA, id: b.id, orderId: null }));
    api.discardDraft.mockResolvedValue(undefined);
    const { result } = renderHook(() => useEnviarCuenta({ onComanda: vi.fn() }), { wrapper });
    await act(async () => { await result.current.enviar('d-1'); });

    const aviso = toasts.create.mock.calls[0][0];
    expect(aviso.action.label).toBe('Empezar cuenta nueva con estos productos');
    await act(async () => { await aviso.action.onClick(); });

    const body = api.createDraft.mock.calls[0][0];
    expect(body.orderId).toBeNull();
    expect(body.id).not.toBe('d-1');
    expect(body.lines).toEqual([{ opId: expect.any(String), productId: 41, qty: '2', modifiers: [{ optionId: 7, qty: 1 }], notes: 'fría' }]);
    await waitFor(() => expect(api.discardDraft).toHaveBeenCalledWith('d-1', 4));
    expect(usePosStore.getState().selected).toEqual({ kind: 'draft', id: body.id });
  });

  test('a un pedido de plataforma lo dice y no ofrece agregar', async () => {
    api.sendDraft.mockRejectedValue(new ApiError(422, 'PLATFORM_ORDER_NO_LINES', 'x', 'r'));
    const { result } = renderHook(() => useEnviarCuenta({ onComanda: vi.fn() }), { wrapper });
    await act(async () => { await result.current.enviar('d-1'); });
    const aviso = toasts.create.mock.calls[0][0];
    expect(aviso.title).toMatch(/plataforma/);
    expect(aviso.action).toBeUndefined();
  });
});
