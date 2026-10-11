import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, expect, test, beforeEach } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { AuditLogTab } from './AuditLogTab';

const list = vi.hoisted(() => vi.fn());
vi.mock('../../api/audit', () => ({
  auditApi: {
    list: (...a: unknown[]) => list(...a),
    actors: () => Promise.resolve([{ id: 7, name: 'Caro' }]),
    orderHistory: () => Promise.resolve([]),
  },
}));
vi.mock('../../hooks/useHoraDelNegocio', () => ({
  useHoraDelNegocio: () => ({ zona: 'America/Mexico_City', diaDelNegocio: () => '2026-10-11' }),
}));

const samplePage = {
  total: 1,
  countsByGroup: { money: 4, cancel: 2, price: 1, cash: 3, attendance: 0 },
  items: [{ id: 1, occurredAt: '2026-10-12T00:22:00Z', action: 'price.set', actorKind: 'user', actorName: 'Caro',
    ip: '201.141.22.9', deviceKey: '7c21aaaabbbbccccddddeeeeffff0000', device: { platform: 'Android', model: 'SM-A145' },
    before: { price: '80' }, after: { price: '85' } }],
};

beforeEach(() => { list.mockReset().mockResolvedValue(samplePage); });

function renderView() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><AuditLogTab /></Provider></QueryClientProvider>);
}

test('los contadores dicen cuántas acciones hubo de cada tipo y la lista habla en español', async () => {
  renderView();
  expect(await screen.findByText('Precio cambiado')).toBeInTheDocument();
  expect(screen.getByText('Precio: $80 → $85')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Cancelaciones y devoluciones: 2' })).toBeInTheDocument();
  expect(screen.queryByText('price.set')).toBeNull();
});

test('tocar un contador filtra por ese tipo y volver a tocarlo lo quita', async () => {
  renderView();
  await screen.findByText('Precio cambiado');
  fireEvent.click(screen.getByRole('button', { name: /^Precios:/ }));
  await waitFor(() => expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ group: 'price', page: 0 })));
  fireEvent.click(screen.getByRole('button', { name: /^Precios:/ }));
  await waitFor(() => expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ group: undefined })));
});

test('el detalle dice desde qué red y aparato se hizo', async () => {
  renderView();
  fireEvent.click(await screen.findByText('Precio cambiado'));
  expect(await screen.findByText('201.141.22.9')).toBeInTheDocument();
  
  expect(screen.getByText('Android · SM-A145 · 7C21')).toBeInTheDocument();
});
