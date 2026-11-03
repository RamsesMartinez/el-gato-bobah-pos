import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, expect, test } from 'vitest';
import { Provider } from '../../components/ui/provider';
import type { SaleRow } from '../../api/sales';
import { SaleDetailDialog } from './SaleDetailDialog';

const perms = vi.hoisted(() => ({ current: [] as string[] }));
vi.mock('../../stores/session', () => ({
  useSessionStore: Object.assign(
    (sel: (s: unknown) => unknown) => sel({ user: { id: 1, role: 'gerente', permissions: perms.current } }),
    { getState: () => ({ user: { id: 1, role: 'gerente', permissions: perms.current } }) },
  ),
}));
vi.mock('../../api/pos', () => ({ posApi: { order: () => Promise.resolve({ lines: [] }) } }));
vi.mock('../../api/audit', () => ({
  auditApi: {
    orderHistory: () => Promise.resolve([
      { id: 1, occurredAt: '2026-09-14T01:40:00Z', action: 'payment.charged', actorKind: 'user', actorName: 'Abi',
        ip: '187.190.4.12', deviceKey: 'a3f9b0c1', after: { amount: '218' } },
      { id: 2, occurredAt: '2026-09-14T01:41:00Z', action: 'payment.voided', actorKind: 'user', actorName: 'Abi',
        ip: '187.190.4.12', deviceKey: 'a3f9b0c1', before: { amount: '218' } },
    ]),
  },
}));
vi.mock('../../hooks/useHoraDelNegocio', () => ({ useHoraDelNegocio: () => ({ zona: 'America/Mexico_City' }) }));

const sale = {
  id: 1179, dailyNumber: 12, date: '2026-09-13', openedAt: '2026-09-14T01:12:00Z', completedAt: null,
  status: 'entregada', serviceType: 'mostrador', total: '218', deliveryFee: '0', tips: '0', openedBy: 'Abi',
  paid: '218', methods: 'Tarjeta', platform: '', platformOrderRef: '', discount: '0', refund: '0',
} as unknown as SaleRow;

function renderView() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider>
    <SaleDetailDialog venta={sale} isOpen onClose={vi.fn()} />
  </Provider></QueryClientProvider>);
}

test('sin permiso no hay pestañas: el detalle se ve como siempre', async () => {
  perms.current = [];
  renderView();
  expect(await screen.findByText('Qué se vendió')).toBeInTheDocument();
  expect(screen.queryByRole('tab', { name: 'Historial' })).toBeNull();
});

test('con permiso, «Historial» cuenta quién cobró y quién devolvió', async () => {
  perms.current = ['audit.view'];
  renderView();
  fireEvent.click(await screen.findByRole('tab', { name: 'Historial' }));
  expect(await screen.findByText('Cobrado')).toBeInTheDocument();
  expect(screen.getByText('Pago devuelto')).toBeInTheDocument();
  expect(screen.queryByText(/187\.190\.4\.12/)).toBeNull();
});
