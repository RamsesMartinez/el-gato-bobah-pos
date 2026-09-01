import { vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';

const openOrders = vi.hoisted(() => vi.fn());
vi.mock('../../api/pos', () => ({ posApi: { openOrders } }));

import { PedidosEnCurso } from './PedidosEnCurso';

function pinta(nodo: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <ChakraProvider value={defaultSystem}>
      <QueryClientProvider client={qc}>{nodo}</QueryClientProvider>
    </ChakraProvider>,
  );
}

const pedido = (over: Record<string, unknown> = {}) => ({
  id: 1, number: 12, folioName: 'Tigre', status: 'abierta', serviceType: 'mostrador',
  deliveryPlatformId: null, customerName: null, total: '250', currency: 'MXN',
  paid: false, outstanding: '250', openedAt: new Date().toISOString(),
  enPreparacion: true, renglones: 3, ...over,
});

afterEach(() => { openOrders.mockReset(); });

// El chip es el camino de UN TOQUE a un pedido en curso. Antes el pedido desaparecía al mandarlo y
// volver a él costaba cinco toques por un camino enterrado en la hoja de cobro — que en producción
// nadie usó jamás.
test('pinta un chip por pedido, con su folio y su monto', async () => {
  openOrders.mockResolvedValue({ items: [pedido()], outstanding: '250' });
  pinta(<PedidosEnCurso onAbrir={() => {}} />);

  expect(await screen.findByText('Tigre')).toBeInTheDocument();
  // El monto del chip y el total en riesgo coinciden con un solo pedido: se buscan los dos.
  expect(screen.getAllByText(/250/).length).toBeGreaterThanOrEqual(1);
  // El chip cumple el mínimo táctil: por debajo el dedo falla y la venta cae en otra mesa.
  expect(screen.getByRole('button', { name: /Tigre/ })).toHaveStyle({ minHeight: '44px' });
});

// Un contador en cero es chrome que le quita ancho a la barra en una tableta de 7".
test('no pinta nada cuando no hay pedidos en curso', async () => {
  openOrders.mockResolvedValue({ items: [], outstanding: '0' });
  const { container } = pinta(<PedidosEnCurso onAbrir={() => {}} />);
  await new Promise((r) => setTimeout(r, 0));
  expect(container.querySelector('button')).toBeNull();
});

// El pedido ENTREGADO y sin cobrar es el caro: el cliente ya se fue con la comida. Se dice en el
// chip y no se deja adivinar — es lo que la píldora que esto reemplaza existía para gritar.
test('el entregado sin cobrar se distingue del que sigue en cocina', async () => {
  openOrders.mockResolvedValue({
    items: [pedido({ id: 2, folioName: 'Lobo', status: 'entregada', enPreparacion: false })],
    outstanding: '250',
  });
  pinta(<PedidosEnCurso onAbrir={() => {}} />);
  expect(await screen.findByText(/ya se entregó/i)).toBeInTheDocument();
});
