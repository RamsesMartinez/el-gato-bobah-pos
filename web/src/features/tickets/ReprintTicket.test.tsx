import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi } from 'vitest';
import { ReprintTicket } from './ReprintTicket';
import type { ReceiptOrder } from '../../types/pos';

const order: ReceiptOrder = {
  folioName: 'Tigre',
  id: 12,
  number: 12,
  status: 'entregada',
  serviceType: 'mostrador',
  customerName: null,
  subtotal: '100',
  deliveryFee: '0',
  total: '100',
  currency: 'MXN',
  paid: true,
  openedAt: '2026-08-27T12:00:00Z',
  lines: [{ productName: 'Ramen', quantity: '1', unitPrice: '100', lineTotal: '100', modifiers: [] }],
};

const getOrder = vi.hoisted(() => vi.fn());
vi.mock('../../api/pos', () => ({ posApi: { order: getOrder } }));

// Se espía la vista previa en vez de renderizarla: lo que hay que probar aquí es QUÉ se le pasa.
const previewProps = vi.hoisted(() => ({ current: null as unknown }));
vi.mock('./TicketPreview', () => ({
  TicketPreview: (props: unknown) => {
    previewProps.current = props;
    return <div data-testid="preview" />;
  },
}));

function wrap(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

beforeEach(() => {
  getOrder.mockReset().mockResolvedValue(order);
  previewProps.current = null;
});

test('pide el pedido COMPLETO: la lista del tablero no trae las líneas', async () => {
  wrap(<ReprintTicket orderId={12} onClose={vi.fn()} />);
  await waitFor(() => expect(getOrder).toHaveBeenCalledWith(12));
  await screen.findByTestId('preview');
  expect((previewProps.current as { order: ReceiptOrder }).order.lines).toHaveLength(1);
});

test('marca el papel como reimpresión', async () => {
  wrap(<ReprintTicket orderId={12} onClose={vi.fn()} />);
  await screen.findByTestId('preview');
  // Sin esto, dos tickets idénticos del mismo pedido pueden circular como ventas distintas.
  expect((previewProps.current as { reprint: boolean }).reprint).toBe(true);
});

test('sin pedido seleccionado no pide nada al servidor', () => {
  wrap(<ReprintTicket orderId={null} onClose={vi.fn()} />);
  expect(getOrder).not.toHaveBeenCalled();
});
