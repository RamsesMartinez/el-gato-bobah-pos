import { render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, beforeEach, test, expect, describe } from 'vitest';
import { MemoryRouter } from 'react-router';
import userEvent from '@testing-library/user-event';
import { Provider } from '../../components/ui/provider';
import { usePosStore } from '../../stores/pos';
import type { AccountItem, DraftView } from '../../types/pos';
import { cuenta } from './__fixtures__/cuentas';
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
  api.paymentMethods.mockResolvedValue({ items: [{ id: 1, name: 'Efectivo', kind: 'efectivo', deliveryPlatformId: null }] });
});

describe('caja', () => {
  test('sin caja abierta no se muestra la pantalla de venta', async () => {
    cashStatus.current = { open: false };
    montar();
    expect(await screen.findByText(/no hay caja abierta/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /cobrar/i })).not.toBeInTheDocument();
  });

  // Aceptar un pedido de plataforma no exige turno: la cocina no espera a que alguien abra caja.
  test('sin caja abierta, un pedido de plataforma sigue avisando', async () => {
    cashStatus.current = { open: false };
    pendientes.current = [{
      id: 1, platformName: 'Uber Eats', displayId: 'K4T2',
      placedAt: '2026-09-17T18:04:00Z', decideBefore: new Date(Date.now() + 600000).toISOString(),
      serviceType: 'domicilio', customerName: 'Ana', total: '342.00', lines: [],
    }];
    montar();
    expect(await screen.findByText(/no hay caja abierta/i)).toBeInTheDocument();
    expect(await screen.findByTestId('aviso-de-pedido-entrante')).toBeInTheDocument();
  });

  test('con caja abierta la pantalla de venta se muestra', async () => {
    montar();
    expect(await screen.findByPlaceholderText(/buscar/i)).toBeInTheDocument();
    expect(screen.queryByText(/no hay caja abierta/i)).not.toBeInTheDocument();
  });

  // Un turno abierto de otro día avisa pero NO bloquea: un negocio prefiere una fecha corrida a una
  // caja parada.
  test('el aviso de turno viejo se ve y NO bloquea la pantalla de venta', async () => {
    cashStatus.current = { open: true, deOtroDia: true, openedAt: '2026-08-31T18:29:00Z' };
    montar();
    expect(await screen.findByText(/la caja lleva abierta desde/i)).toBeInTheDocument();
    expect(await screen.findByPlaceholderText(/buscar/i)).toBeInTheDocument();
  });

  test('un turno de hoy, o sin el campo del servidor, no muestra el aviso', async () => {
    cashStatus.current = { open: true, deOtroDia: false };
    montar();
    expect(await screen.findByPlaceholderText(/buscar/i)).toBeInTheDocument();
    expect(screen.queryByText(/la caja lleva abierta desde/i)).not.toBeInTheDocument();
  });
});

describe('una sola fila de cuentas (US1)', () => {
  // Las pestañas «Cuenta 1 · 2» vivían solo en una tableta y el botón naranja era otra puerta para
  // lo mismo. Las dos se fueron: la fila es la única.
  test('no quedan las pestañas locales ni el botón de pedidos por cobrar', async () => {
    vivas.current = [cuenta({ key: 'o:1', orderId: 1 })];
    montar();
    expect(await screen.findByRole('button', { name: /^Khao Manee ·/ })).toBeInTheDocument();
    expect(screen.queryByText(/Cuenta 1/)).toBeNull();
    expect(screen.queryByRole('button', { name: /por cobrar/i })).toBeNull();
  });

  // Con 10 cuentas la fila pinta solo las fichas completas y cuenta las demás: nada se desborda ni
  // se esconde detrás de un scroll horizontal.
  test('con 10 cuentas pinta las que caben y «+N» cuenta exactamente las demás', async () => {
    vivas.current = Array.from({ length: 10 }, (_, i) => cuenta({ key: `o:${i + 1}`, orderId: i + 1, folioName: `Gato ${i + 1}` }));
    montar();
    const fila = await screen.findByLabelText('Cuentas');
    await waitFor(() => expect(within(fila).getAllByRole('button').filter((b) => b.dataset.ficha === 'true').length).toBeGreaterThan(0));
    const fichas = within(fila).getAllByRole('button').filter((b) => b.dataset.ficha === 'true').length;
    expect(within(fila).getByRole('button', { name: new RegExp(`\\(${10 - fichas} más\\)`) })).toBeInTheDocument();
  });

  test('tocar una ficha la abre en el ticket', async () => {
    vivas.current = [cuenta({ key: 'd:d-1', kind: 'draft', draftId: 'd-1', orderId: null, folioName: 'Levkoy', state: 'capturing', group: 'capturing' })];
    api.getDraft.mockResolvedValue(draft());
    anchoDelPos.width = 1024;
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /^Levkoy ·/ }));
    expect(usePosStore.getState().selected).toEqual({ kind: 'draft', id: 'd-1' });
    expect(await screen.findByText('Crepa')).toBeInTheDocument();
  });

  // Con el panel abierto la fila tiene ~612 px: el buscador cede su ancho a las cuentas y se
  // despliega al tocarlo.
  test('con el panel abierto el buscador se pliega a un botón que despliega el campo', async () => {
    anchoDelPos.width = 1024;
    montar();
    const lupa = await screen.findByRole('button', { name: 'Buscar producto' });
    expect(screen.queryByPlaceholderText(/buscar/i)).toBeNull();
    await userEvent.click(lupa);
    expect(screen.getByPlaceholderText(/buscar/i)).toBeInTheDocument();
  });
});

// LAS SUPERFICIES QUE PINTAN EL TOTAL TIENEN QUE DECIR LO MISMO: el total del SERVIDOR, con su
// descuento. A 1024×600 el panel arranca colapsado y la píldora es la que se ve todo el día.
test.each([500, 1024])('la píldora y la barra pintan el total del servidor, con descuento (ancho %i)', async (ancho) => {
  anchoDelPos.width = ancho;
  tabletaBaja(true);
  usePosStore.getState().seleccionar({ kind: 'draft', id: 'd-1' });
  api.getDraft.mockResolvedValue(draft());
  montar();
  await waitFor(() => expect(screen.getAllByText(/art ·/).length).toBeGreaterThan(0));
  for (const s of screen.getAllByText(/art ·/)) {
    expect(s.textContent, 'esta superficie no pinta el total que cobra el servidor').toContain('$75');
  }
});

// El menú de Chakra se porta a otro nodo: si sobreviviera al panel que lo contiene, quedaría
// flotando sobre el catálogo sin dueño.
test('el menú de la cuenta no queda flotando cuando el panel se va', async () => {
  anchoDelPos.width = 1024;
  usePosStore.getState().seleccionar({ kind: 'draft', id: 'd-1' });
  api.getDraft.mockResolvedValue(draft());
  const { rerender } = montar();
  await screen.findByText('Crepa');
  await userEvent.click(await screen.findByRole('button', { name: 'Más opciones de la cuenta' }));
  expect(await screen.findByRole('menuitem', { name: /Descuento/ })).toBeInTheDocument();
  anchoDelPos.width = 500;
  rerender(montarArbol());
  expect(screen.queryByRole('menuitem', { name: /Descuento/ })).toBeNull();
});
