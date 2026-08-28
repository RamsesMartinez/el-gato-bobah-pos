import { render } from '@testing-library/react';
import { vi } from 'vitest';
import { AutoPrintTicket } from './AutoPrintTicket';
import type { OrderView } from '../../types/pos';

const printHtmlOffscreen = vi.hoisted(() => vi.fn((_html: string) => Promise.resolve(true)));
vi.mock('../../utils/printReceipt', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../utils/printReceipt')>()),
  printHtmlOffscreen,
}));

const info = vi.hoisted(() => ({
  current: { autoPrintOnClose: false } as { autoPrintOnClose: boolean },
}));
vi.mock('./ticketBusinessInfo', () => ({
  useTicketBusinessInfo: () => ({
    data: {
      businessName: 'El Gato Bobah',
      address: '',
      phone: '',
      headerNote: '',
      footerNote: '',
      logoDataUri: 'data:image/png;base64,QUFB',
    },
    isLoading: false,
    autoPrintOnClose: info.current.autoPrintOnClose,
  }),
}));

const order: OrderView = {
  id: 7,
  number: 7,
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

beforeEach(() => printHtmlOffscreen.mockClear());

test('apagado no imprime nada', () => {
  info.current.autoPrintOnClose = false;
  render(<AutoPrintTicket order={order} />);
  expect(printHtmlOffscreen).not.toHaveBeenCalled();
});

test('encendido imprime el ticket del pedido recién cerrado', () => {
  info.current.autoPrintOnClose = true;
  render(<AutoPrintTicket order={order} />);
  expect(printHtmlOffscreen).toHaveBeenCalledTimes(1);
  expect(printHtmlOffscreen.mock.calls[0][0]).toContain('El Gato Bobah');
  expect(printHtmlOffscreen.mock.calls[0][0]).toContain('Ramen');
});

test('un re-render del mismo pedido no saca un segundo ticket', () => {
  info.current.autoPrintOnClose = true;
  const { rerender } = render(<AutoPrintTicket order={order} />);
  rerender(<AutoPrintTicket order={order} />);
  rerender(<AutoPrintTicket order={{ ...order }} />); // objeto nuevo, mismo pedido
  expect(printHtmlOffscreen).toHaveBeenCalledTimes(1);
});

test('el siguiente pedido sí imprime', () => {
  info.current.autoPrintOnClose = true;
  const { rerender } = render(<AutoPrintTicket order={order} />);
  rerender(<AutoPrintTicket order={{ ...order, id: 8, number: 8 }} />);
  expect(printHtmlOffscreen).toHaveBeenCalledTimes(2);
});

test('sin pedido no imprime', () => {
  info.current.autoPrintOnClose = true;
  render(<AutoPrintTicket order={null} />);
  expect(printHtmlOffscreen).not.toHaveBeenCalled();
});
