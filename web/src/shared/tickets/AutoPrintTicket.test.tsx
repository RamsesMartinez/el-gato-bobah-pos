import { render, waitFor } from '@testing-library/react';
import { vi } from 'vitest';
import { AutoPrintTicket } from './AutoPrintTicket';
import type { ReceiptOrder } from '../../types/pos';

const printHtmlOffscreen = vi.hoisted(() => vi.fn((_html: string) => Promise.resolve(true)));
vi.mock('../../utils/printReceipt', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../utils/printReceipt')>()),
  printHtmlOffscreen,
}));

const toast = vi.hoisted(() => vi.fn());
vi.mock('../../components/ui/toaster', () => ({ toaster: { create: toast } }));

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

const order: ReceiptOrder = {
  folioName: 'Tigre',
  id: 7,
  number: 7,
  status: 'entregada',
  serviceType: 'mostrador',
  customerName: null,
  subtotal: '100',
  discount: '0',
  deliveryFee: '0',
  total: '100',
  currency: 'MXN',
  paid: true,
  openedAt: '2026-08-27T12:00:00Z',
  lines: [{ productName: 'Ramen', quantity: '1', unitPrice: '100', lineTotal: '100', modifiers: [] }],
};

beforeEach(() => {
  printHtmlOffscreen.mockClear();
  printHtmlOffscreen.mockResolvedValue(true);
  toast.mockClear();
});

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

test('si la impresión no sale, el operador se entera', async () => {
  // El modo de fallo que costó esta ronda: no salía nada y no había forma de saberlo. Un aviso
  // convierte el silencio en algo accionable — el ticket sigue disponible en "Ver ticket".
  info.current.autoPrintOnClose = true;
  printHtmlOffscreen.mockResolvedValueOnce(false);
  render(<AutoPrintTicket order={order} />);
  await waitFor(() => expect(toast).toHaveBeenCalled());
  expect(String(toast.mock.calls[0][0].title)).toMatch(/no se pudo imprimir/i);
});

// Pedido dividido (spec 027): cada pago saca SU ticket, y se recuerda por id de pago. Recordando
// por pedido, el segundo comensal se quedaría sin papel; sin recordar, cada re-render imprimiría
// otra vez el del primero.
describe('pedido dividido', () => {
  const pago = (id: number, number: number, extra: object = {}) => ({
    id, number, voided: false, methodId: 9, methodName: 'Efectivo', amount: '50.00', tip: '0.00',
    reference: '', paidAt: '2026-10-04T21:23:28-06:00', receivedBy: 'carlos', split: null,
    lines: [], ...extra,
  });
  const dividido = {
    ...order, paid: false, outstanding: '50.00',
    lines: [{ id: 1, productName: 'Ramen', quantity: '2', unitPrice: '50', lineTotal: '100', modifiers: [] }],
    payments: [pago(201, 1, { lines: [{ lineId: 1, qty: '1', amount: '50.00' }] })],
  };

  test('el primer pago saca el ticket de ESE pago, no el del pedido', () => {
    info.current.autoPrintOnClose = true;
    render(<AutoPrintTicket order={dividido} />);
    expect(printHtmlOffscreen).toHaveBeenCalledTimes(1);
    const html = printHtmlOffscreen.mock.calls[0][0];
    expect(html).toContain('Pago 1');
    expect(html).toContain('1x Ramen');
    expect(html).toContain('Del pedido quedan por pagar');
  });

  test('el siguiente pago del MISMO pedido también imprime, y solo el suyo', () => {
    info.current.autoPrintOnClose = true;
    const { rerender } = render(<AutoPrintTicket order={dividido} />);
    rerender(<AutoPrintTicket order={{
      ...dividido, paid: true, outstanding: '0.00',
      payments: [...dividido.payments, pago(202, 2, { lines: [{ lineId: 1, qty: '1', amount: '50.00' }] })],
    }} />);
    expect(printHtmlOffscreen).toHaveBeenCalledTimes(2);
    expect(printHtmlOffscreen.mock.calls[1][0]).toContain('Pago 2');
    expect(printHtmlOffscreen.mock.calls[1][0]).not.toContain('Pago 1');
  });

  test('un re-render con los mismos pagos no saca un segundo ticket', () => {
    info.current.autoPrintOnClose = true;
    const { rerender } = render(<AutoPrintTicket order={dividido} />);
    rerender(<AutoPrintTicket order={{ ...dividido }} />);
    expect(printHtmlOffscreen).toHaveBeenCalledTimes(1);
  });

  test('un pago devuelto no imprime', () => {
    info.current.autoPrintOnClose = true;
    render(<AutoPrintTicket order={{
      ...dividido,
      payments: [pago(201, 1, { voided: true }), pago(202, 2)],
    }} />);
    expect(printHtmlOffscreen).toHaveBeenCalledTimes(1);
    expect(printHtmlOffscreen.mock.calls[0][0]).toContain('Pago 2');
  });

  test('apagado no imprime ningún pago', () => {
    info.current.autoPrintOnClose = false;
    render(<AutoPrintTicket order={dividido} />);
    expect(printHtmlOffscreen).not.toHaveBeenCalled();
  });

  test('un pedido pagado de una sola vez sigue sacando el ticket completo', () => {
    info.current.autoPrintOnClose = true;
    render(<AutoPrintTicket order={{ ...order, outstanding: '0.00', payments: [pago(300, 1, { amount: '100.00' })] }} />);
    const html = printHtmlOffscreen.mock.calls[0][0];
    expect(html).not.toContain('Pago 1');
    expect(html).toContain('PAGADO');
  });
});
