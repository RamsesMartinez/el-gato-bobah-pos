import { act, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, beforeEach, test, expect, describe } from 'vitest';
import { MemoryRouter } from 'react-router';
import userEvent from '@testing-library/user-event';
import { Provider } from '../../components/ui/provider';
import { usePosStore } from '../../stores/pos';
import type { AccountItem, DraftView, OrderView } from '../../types/pos';
import { ApiError } from '../../api/client';
import { reportarResultado } from './useSinConexion';
import { reiniciarCaptura } from './useCuenta';
import { POSPage } from './POSPage';

const pendientes = vi.hoisted(() => ({ current: [] as unknown[] }));
vi.mock('../../api/pedidosDePlataforma', () => ({
  pedidosPendientes: () => Promise.resolve(pendientes.current),
  aceptarPedidoDePlataforma: vi.fn(),
}));

const cashStatus = vi.hoisted(() => ({
  current: { open: true } as { open: boolean; deOtroDia?: boolean; openedAt?: string },
}));
const vivas = vi.hoisted(() => ({ current: [] as AccountItem[] }));
const api = vi.hoisted(() => ({
  getDraft: vi.fn(),
  order: vi.fn(),
  sendDraft: vi.fn(),
  paymentMethods: vi.fn(),
}));
vi.mock('../../api/pos', () => ({
  posApi: {
    cashStatus: () => Promise.resolve(cashStatus.current),
    menu: () => Promise.resolve({ categories: [], products: [] }),
    popular: () => Promise.resolve({ items: [] }),
    modifierDefaults: () => Promise.resolve({}),
    businessSettings: () => Promise.resolve({ deliveryFee: '20', timezone: 'America/Mexico_City' }),
    folioNames: () => Promise.resolve({ items: ['Levkoy'] }),
    liveAccounts: () => Promise.resolve({ items: vivas.current, outstanding: '0.00', serverTime: '2026-10-08T16:00:00Z' }),
    quoteOrder: vi.fn(),
    ...api,
  },
}));
vi.mock('../../hooks/useMenu', () => ({
  useMenu: () => ({ data: { categories: [], products: [], platforms: [] }, isLoading: false, error: null }),
}));
vi.mock('../../hooks/usePopular', () => ({ usePopular: () => ({ data: [] }) }));
// EL ANCHO SE MOCKEA PORQUE JSDOM NO HACE LAYOUT: sin esto `wide` sería siempre false y la rama
// ancha del POS —donde viven la píldora y el panel— no se montaría nunca.
const anchoDelPos = vi.hoisted(() => ({ width: 500 }));
function tabletaBaja(baja: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: baja && query.includes('max-height'),
    media: query,
    onchange: null,
    addEventListener: () => {}, removeEventListener: () => {},
    addListener: () => {}, removeListener: () => {}, dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}
vi.mock('../../hooks/useContainerWidth', () => ({
  useContainerWidth: () => ({ ref: { current: null }, width: anchoDelPos.width }),
}));
vi.mock('../../hooks/useModifierDefaults', () => ({ useModifierDefaults: () => ({ data: {} }) }));

function draft(over: Partial<DraftView> = {}): DraftView {
  return {
    id: 'd-1', orderId: null, folioName: 'Levkoy', status: 'capturando', headerVersion: 1,
    updatedAt: '', createdAt: '', openedBy: 'Ana', serviceType: 'mostrador', customerName: null,
    platformId: null, platformOrderRef: null, deliveryFee: '0.00', discount: { amount: '20.00' },
    lines: [{ id: 'l-1', version: 1, productId: 1, productName: 'Crepa', qty: '1', unitPrice: '95.00', modifiers: [], notes: '', lineTotal: '95.00', available: true }],
    subtotal: '95.00', discountTotal: '20.00', total: '75.00', unavailable: [], ...over,
  };
}

function montarArbol() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={qc}>
      <Provider>
        <MemoryRouter>
          <POSPage />
        </MemoryRouter>
      </Provider>
    </QueryClientProvider>
  );
}
const montar = () => render(montarArbol());

beforeEach(() => {
  vi.clearAllMocks();
  reiniciarCaptura();
  localStorage.clear();
  anchoDelPos.width = 500;
  tabletaBaja(false);
  cashStatus.current = { open: true };
  pendientes.current = [];
  vivas.current = [];
  usePosStore.setState({ selected: null });
  usePosStore.getState().reiniciarNueva();
  anchoDelPos.width = 1024;
  reportarResultado(null);
  api.paymentMethods.mockResolvedValue({ items: [{ id: 1, name: 'Efectivo', kind: 'efectivo', deliveryPlatformId: null }] });
});

const PEDIDO = {
  id: 30, number: 7, folioName: 'Levkoy', status: 'abierta', serviceType: 'mostrador', deliveryPlatformId: null,
  platformOrderRef: null, customerName: null, subtotal: '95.00', discount: '20.00', deliveryFee: '0.00',
  total: '75.00', currency: 'MXN', paid: false, outstanding: '75.00', openedAt: '',
  lines: [{ id: 501, productName: 'Crepa', quantity: '1', unitPrice: '95.00', lineTotal: '95.00', delivered: '0', cancelled: false }],
  payments: [], canSplit: true,
} as OrderView;

// UNA SOLA PUERTA PARA COBRAR (US4, research R-5).
describe('cobrar desde el ticket', () => {
  test('con algo nuevo dice «Enviar y cobrar», manda a cocina y DESPUÉS abre la hoja', async () => {
    usePosStore.getState().seleccionar({ kind: 'draft', id: 'd-1' });
    api.getDraft.mockResolvedValue(draft());
    api.order.mockResolvedValue(PEDIDO);
    let soltar: () => void = () => {};
    api.sendDraft.mockImplementation(() => new Promise((r) => { soltar = () => r({ order: PEDIDO, printLineIds: [501], created: true }); }));
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /Enviar y cobrar \$75/ }));
    expect(api.sendDraft).toHaveBeenCalledWith('d-1');
    // Mientras el envío no regresa, la hoja no se abre: no hay pedido sobre el cual cobrar.
    expect(screen.queryByRole('button', { name: 'Efectivo' })).toBeNull();
    await act(async () => soltar());
    expect(await screen.findByRole('button', { name: 'Efectivo' })).toBeInTheDocument();
    expect(usePosStore.getState().selected).toEqual({ kind: 'order', id: 30 });
  });

  test('sin nada nuevo dice «Cobrar» y abre la hoja sin mandar nada', async () => {
    usePosStore.getState().seleccionar({ kind: 'order', id: 30 });
    api.order.mockResolvedValue(PEDIDO);
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /^Cobrar \$75/ }));
    expect(await screen.findByRole('button', { name: 'Efectivo' })).toBeInTheDocument();
    expect(api.sendDraft).not.toHaveBeenCalled();
  });

  test('si el envío falla la hoja no se abre y el motivo queda en el pie', async () => {
    usePosStore.getState().seleccionar({ kind: 'draft', id: 'd-1' });
    api.getDraft.mockResolvedValue(draft());
    api.sendDraft.mockRejectedValue(new ApiError(409, 'NO_OPEN_REGISTER', 'x', 'r'));
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /Enviar y cobrar/ }));
    expect(await screen.findByText('No hay caja abierta')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Efectivo' })).toBeNull();
    expect(usePosStore.getState().selected).toEqual({ kind: 'draft', id: 'd-1' });
  });

  test('sin conexión cobrar está apagado', async () => {
    usePosStore.getState().seleccionar({ kind: 'draft', id: 'd-1' });
    api.getDraft.mockResolvedValue(draft());
    montar();
    await screen.findByText('Crepa');
    act(() => reportarResultado(new TypeError('Failed to fetch')));
    expect(screen.getByRole('button', { name: /Enviar y cobrar/ })).toBeDisabled();
    expect(screen.getByRole('alert')).toHaveTextContent('Sin conexión');
  });
});
