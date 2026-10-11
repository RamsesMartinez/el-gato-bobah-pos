import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, expect, test } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { OrderHistoryTimeline } from './OrderHistoryTimeline';

const history = vi.hoisted(() => ({
  10: [{ id: 1, occurredAt: '2026-10-10T20:00:00Z', action: 'order.lines_moved', actorKind: 'user', actorName: 'Abi',
    after: { toOrderId: 20, lines: 2 } }],
  20: [{ id: 2, occurredAt: '2026-10-10T20:00:00Z', action: 'order.lines_moved', actorKind: 'user', actorName: 'Abi',
    after: { fromOrderId: 10, lines: 2 } }],
} as Record<number, unknown[]>));
vi.mock('../../api/audit', () => ({ auditApi: { orderHistory: (id: number) => Promise.resolve(history[id] ?? []) } }));
vi.mock('../../hooks/useHoraDelNegocio', () => ({ useHoraDelNegocio: () => ({ zona: 'America/Mexico_City' }) }));

function renderView(orderId: number) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><OrderHistoryTimeline orderId={orderId} /></Provider></QueryClientProvider>);
}

// US3/AC2: el traslado se ve en los DOS pedidos y lleva al otro; sin eso, quien audita un pedido al
// que le faltan productos no sabe a dónde fueron.
test('el traslado dice a dónde se fue y abre el historial del otro pedido', async () => {
  renderView(10);
  expect(await screen.findByText(/Se pasaron 2 productos a otro pedido/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Ver el otro pedido' }));
  expect(await screen.findByText(/Llegaron 2 productos de otro pedido/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Volver al pedido anterior' }));
  expect(await screen.findByText(/Se pasaron 2 productos a otro pedido/)).toBeInTheDocument();
});

// Se llega al traslado bajando por un historial largo: el otro pedido tiene que abrir desde arriba,
// donde está «Volver», y no a media hoja.
test('abrir el otro pedido lleva la hoja al inicio', async () => {
  const scroll = vi.fn();
  Element.prototype.scrollIntoView = scroll;
  renderView(10);
  fireEvent.click(await screen.findByRole('button', { name: 'Ver el otro pedido' }));
  await screen.findByText(/Llegaron 2 productos/);
  expect(scroll).toHaveBeenCalled();
});
