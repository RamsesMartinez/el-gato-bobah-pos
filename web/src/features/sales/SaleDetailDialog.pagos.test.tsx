import { render, screen, fireEvent, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, expect, test, beforeEach } from 'vitest';

import { Provider } from '../../components/ui/provider';
import type { SaleRow } from '../../api/sales';
import { SaleDetailDialog } from './SaleDetailDialog';

const perms = vi.hoisted(() => ({ current: [] as string[] }));
const cardTerminals = vi.hoisted(() => vi.fn());
const t = (id: number, name: string, archived = false) => ({ id, branchId: 1, branchName: 'Matriz', name, archived });
vi.mock('../../stores/session', () => ({
  useSessionStore: Object.assign(
    (sel: (s: unknown) => unknown) => sel({ user: { id: 1, role: 'gerente', permissions: perms.current } }),
    { getState: () => ({ user: { id: 1, role: 'gerente', permissions: perms.current } }) },
  ),
}));
const base = { voided: false, tip: '0', reference: '', paidAt: '2026-10-11T20:00:00Z', receivedBy: 'Ana', split: null, lines: [] };
vi.mock('../../api/pos', () => ({
  posApi: {
    order: () => Promise.resolve({
      lines: [],
      payments: [
        { ...base, id: 77, number: 1, methodId: 3, methodName: 'Tarjeta débito', methodKind: 'tarjeta', amount: '250', terminalId: 2, terminalName: 'Terminal A' },
        { ...base, id: 78, number: 2, methodId: 4, methodName: 'Tarjeta crédito', methodKind: 'tarjeta', amount: '20', terminalId: null, terminalName: null },
        { ...base, id: 79, number: 3, methodId: 1, methodName: 'Efectivo', methodKind: 'efectivo', amount: '28', terminalId: null, terminalName: null },
      ],
    }),
    cardTerminals,
  },
}));
vi.mock('../../hooks/useHoraDelNegocio', () => ({ useHoraDelNegocio: () => ({ zona: 'America/Mexico_City' }) }));

const sale = {
  id: 1179, dailyNumber: 2, date: '2026-10-11', openedAt: '2026-10-11T19:50:00Z', completedAt: null,
  status: 'entregada', serviceType: 'mostrador', total: '298', deliveryFee: '0', tips: '0', openedBy: 'Ana',
  paid: '298', methods: 'Tarjeta débito, Tarjeta crédito, Efectivo', platform: '', platformOrderRef: '', discount: '0', refund: '0',
} as unknown as SaleRow;

beforeEach(() => {
  cardTerminals.mockReset().mockResolvedValue({ items: [t(2, 'Terminal A'), t(3, 'Terminal B'), t(4, 'Terminal C', true)] });
});

function renderView() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider>
    <SaleDetailDialog venta={sale} isOpen onClose={vi.fn()} />
  </Provider></QueryClientProvider>);
}

test('«Pagos» lista cada pago con su terminal y reemplaza a «Medio de pago»', async () => {
  perms.current = [];
  renderView();
  const pagos = await screen.findByRole('list', { name: 'Pagos' });
  const items = await within(pagos).findAllByRole('listitem');
  expect(items).toHaveLength(3);
  expect(items[0]).toHaveTextContent('Tarjeta débito · Terminal A');
  expect(items[0]).toHaveTextContent('$250');
  expect(items[1]).toHaveTextContent('Tarjeta crédito · Sin terminal');
  expect(items[2]).toHaveTextContent('Efectivo');
  expect(items[2]).not.toHaveTextContent('terminal');
  expect(screen.queryByText('Medio de pago')).toBeNull();
});

test('sin el permiso no se ofrece cambiar la terminal', async () => {
  perms.current = ['audit.view'];
  renderView();
  await screen.findAllByRole('listitem');
  expect(screen.queryByRole('button', { name: /Cambiar terminal/ })).toBeNull();
});

test('con el permiso, solo los pagos con tarjeta ofrecen cambiarla, y abre la hoja', async () => {
  perms.current = ['payments.change_terminal'];
  renderView();
  const botones = await screen.findAllByRole('button', { name: /Cambiar terminal/ });
  expect(botones).toHaveLength(2);
  expect(botones[0]).toHaveStyle({ minHeight: '44px' });
  fireEvent.click(botones[0]);
  expect(await screen.findByText(/Ahora: Terminal A/)).toBeInTheDocument();
});

// Con una sola terminal activa no hay a dónde mover el pago: la hoja abriría con la única terminal
// deshabilitada y Confirmar apagado para siempre.
test('con una sola terminal activa no se ofrece cambiarla', async () => {
  perms.current = ['payments.change_terminal'];
  cardTerminals.mockResolvedValue({ items: [t(2, 'Terminal A'), t(3, 'Terminal B', true)] });
  renderView();
  await screen.findAllByRole('listitem');
  await new Promise((r) => setTimeout(r, 50));
  expect(screen.queryByRole('button', { name: /Cambiar terminal/ })).toBeNull();
});

// Solo el renglón con botón necesita 44 px; los demás no se tocan y empujaban «Qué se vendió» fuera
// del alto de la tableta.
test('el renglón sin botón no ocupa el alto de un control', async () => {
  perms.current = ['payments.change_terminal'];
  renderView();
  await screen.findAllByRole('button', { name: /Cambiar terminal/ });
  const items = screen.getAllByRole('listitem');
  expect(items[0]).toHaveStyle({ minHeight: '44px' });
  expect(items[2]).not.toHaveStyle({ minHeight: '44px' });
});

// Dos pagos con tarjeta dan dos botones iguales uno encima del otro: con 4 px entre ellos el dedo
// cae en el de al lado.
test('entre renglones de pago hay al menos 8 px', async () => {
  perms.current = ['payments.change_terminal'];
  renderView();
  const lista = await screen.findByRole('list', { name: 'Pagos' });
  expect(getComputedStyle(lista).gap).toMatch(/spacing-2|0\.5rem|8px/);
});
