import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
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

// FR-016: el filtro ofrece los aparatos de todo el periodo, no solo los de la página que se ve.
test('el filtro «Aparato» ofrece también los que no salen en esta página', async () => {
  list.mockResolvedValue({ ...samplePage, deviceKeys: ['7c21aaaabbbbccccddddeeeeffff0000', '9e04aaaabbbbccccddddeeeeffff0000'] });
  renderView();
  await screen.findByText('Precio cambiado');
  fireEvent.click(screen.getByRole('button', { name: /Más filtros/ }));
  fireEvent.click(await screen.findByRole('button', { name: /Todos los aparatos/ }));
  expect(await screen.findByText('Aparato 9E04')).toBeInTheDocument();
});

// SIN RED, UN FILTRO NUEVO NO SE QUEDA CON LA LISTA VIEJA. Tocar «Ayer» sin red marcaba «Ayer» y
// seguía mostrando lo de hoy, sin aviso: la pantalla contestaba otra pregunta.
test('sin red, cambiar el filtro quita la lista y dice «Sin conexión»', async () => {
  renderView();
  await screen.findByText('Precio cambiado');
  onlineManager.setOnline(false);
  try {
    list.mockRejectedValue(new ApiError(0, 'NETWORK', 'x', 'r1'));
    fireEvent.click(screen.getByRole('button', { name: 'Ayer' }));
    expect(await screen.findByText('Sin conexión', {}, { timeout: 3000 })).toBeInTheDocument();
    expect(screen.queryByText('Precio cambiado')).toBeNull();
    expect(screen.getByRole('button', { name: 'Cancelaciones y devoluciones: 0' })).toBeInTheDocument();
  } finally {
    onlineManager.setOnline(true);
  }
});

// «Cobros 5» contaba también «Pedido creado» y «Entregado»: el contador no decía lo que contaba.
test('el contador del dinero no se llama «Cobros»', async () => {
  renderView();
  await screen.findByText('Precio cambiado');
  expect(screen.queryByText('Cobros')).toBeNull();
  expect(screen.getByRole('button', { name: 'Pedidos y cobros: 4' })).toBeInTheDocument();
});

// Cada renglón de pedido dice de qué pedido es, igual que Ventas.
test('el renglón de un pedido lo nombra como Ventas', async () => {
  list.mockResolvedValue({ ...samplePage, items: [{ ...samplePage.items[0], action: 'payment.charged', orderId: 9,
    orderLabel: 'Selkirk Rex · #12', before: { paid: '0' }, after: { amount: '63', paid: '63' } }] });
  renderView();
  expect(await screen.findByText(/Selkirk Rex · #12/)).toBeInTheDocument();
});
