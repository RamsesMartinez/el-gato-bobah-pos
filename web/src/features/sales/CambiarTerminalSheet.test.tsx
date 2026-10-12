import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, expect, test, beforeEach } from 'vitest';

import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import type { PaymentView } from '../../types/pos';
import { CambiarTerminalSheet } from './CambiarTerminalSheet';

const api = vi.hoisted(() => ({
  cardTerminals: vi.fn(),
  changePaymentTerminal: vi.fn(),
}));
vi.mock('../../api/pos', () => ({ posApi: api }));

const terminales = [
  { id: 2, branchId: 1, branchName: 'Matriz', name: 'Terminal A', archived: false },
  { id: 3, branchId: 1, branchName: 'Matriz', name: 'Terminal B', archived: false },
  { id: 4, branchId: 1, branchName: 'Matriz', name: 'Terminal C', archived: true },
];

// Un débito capturados en Terminal A y cobrados en Terminal B.
const pago = {
  id: 77, number: 1, voided: false, methodId: 3, methodName: 'Tarjeta débito', methodKind: 'tarjeta', amount: '250', tip: '0',
  reference: '', paidAt: '2026-10-11T20:00:00Z', receivedBy: 'Ana', split: null, lines: [],
  terminalId: 2, terminalName: 'Terminal A',
} as PaymentView;

beforeEach(() => {
  api.cardTerminals.mockReset().mockResolvedValue({ items: terminales });
  api.changePaymentTerminal.mockReset().mockResolvedValue({ id: 1179, payments: [] });
});

function renderSheet(p: PaymentView = pago) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidate = vi.spyOn(qc, 'invalidateQueries');
  const onCerrar = vi.fn();
  render(<QueryClientProvider client={qc}><Provider>
    <CambiarTerminalSheet orderId={1179} pago={p} onCerrar={onCerrar} />
  </Provider></QueryClientProvider>);
  return { invalidate, onCerrar };
}

test('ofrece las activas con botones grandes; la actual no se puede elegir y la archivada no sale', async () => {
  renderSheet();
  const termB = await screen.findByRole('button', { name: 'Terminal B' });
  expect(termB).toHaveStyle({ minHeight: '52px' });
  expect(screen.getByRole('button', { name: /Terminal A/ })).toBeDisabled();
  expect(screen.queryByRole('button', { name: /Terminal C/ })).toBeNull();
});

test('el caso común son tres toques: el motivo llega escrito y sin abrir el teclado', async () => {
  renderSheet();
  const motivo = await screen.findByLabelText('Motivo');
  expect(motivo).toHaveValue('Se cobró en otra terminal');
  expect(motivo).not.toHaveFocus();
  const confirmar = screen.getByRole('button', { name: 'Confirmar' });
  expect(confirmar).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: 'Terminal B' }));
  expect(confirmar).toBeEnabled();
  fireEvent.change(motivo, { target: { value: 'ab' } });
  expect(confirmar).toBeDisabled();
  fireEvent.change(motivo, { target: { value: 'Se cobró en otra terminal' } });
  fireEvent.click(confirmar);
  await waitFor(() => expect(api.changePaymentTerminal).toHaveBeenCalledWith(1179, 77,
    { fromTerminalId: 2, terminalId: 3, reason: 'Se cobró en otra terminal' }));
});

test('un pago sin terminal lo dice y ofrece todas las activas', async () => {
  renderSheet({ ...pago, terminalId: null, terminalName: null });
  expect(await screen.findByText(/Sin terminal/)).toBeInTheDocument();
  expect(await screen.findByRole('button', { name: 'Terminal A' })).toBeEnabled();
  fireEvent.click(screen.getByRole('button', { name: 'Terminal B' }));
  fireEvent.click(screen.getByRole('button', { name: 'Confirmar' }));
  await waitFor(() => expect(api.changePaymentTerminal).toHaveBeenCalledWith(1179, 77,
    expect.objectContaining({ fromTerminalId: null, terminalId: 3 })));
});

test('al confirmar se recargan la venta, la lista y la caja, y la hoja se cierra', async () => {
  const { invalidate, onCerrar } = renderSheet();
  fireEvent.click(await screen.findByRole('button', { name: 'Terminal B' }));
  fireEvent.click(screen.getByRole('button', { name: 'Confirmar' }));
  await waitFor(() => expect(onCerrar).toHaveBeenCalled());
  const keys = invalidate.mock.calls.map((c) => JSON.stringify(c[0]?.queryKey));
  expect(keys).toEqual(expect.arrayContaining(['["orders",1179]', '["sales"]', '["cash"]']));
});

test('si otro cambio llegó antes, lo dice y recarga la venta en vez de cerrar', async () => {
  api.changePaymentTerminal.mockRejectedValue(new ApiError(409, 'CONFLICT', 'el pago ya cambió', 'x'));
  const { invalidate, onCerrar } = renderSheet();
  fireEvent.click(await screen.findByRole('button', { name: 'Terminal B' }));
  fireEvent.click(screen.getByRole('button', { name: 'Confirmar' }));
  expect(await screen.findByText(/El pago ya cambió/)).toBeInTheDocument();
  expect(invalidate.mock.calls.map((c) => JSON.stringify(c[0]?.queryKey))).toContain('["orders",1179]');
  expect(onCerrar).not.toHaveBeenCalled();
});

// Con muchas terminales los botones bajarían el motivo y Confirmar fuera de la vista: la misma regla
// que en el cobro, botones hasta 4 y la lista a partir de 5.
test('con cinco o más terminales se elige en la lista, no en botones', async () => {
  const t = (id: number, name: string) => ({ id, branchId: 1, branchName: 'Matriz', name, archived: false });
  api.cardTerminals.mockResolvedValue({ items: [t(2, 'Terminal A'), t(3, 'B'), t(5, 'C'), t(6, 'D'), t(7, 'E')] });
  renderSheet();
  expect(await screen.findByRole('button', { name: /Elige la terminal/ })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'B' })).toBeNull();
});

// Un motivo prellenado que no es el que pasó queda falso en la bitácora. Los dos de siempre se
// eligen con un toque; el primero llega marcado.
test('el motivo se elige entre los dos de siempre y se puede escribir otro', async () => {
  renderSheet();
  const otra = await screen.findByRole('button', { name: 'Se cobró en otra terminal' });
  expect(otra).toHaveAttribute('aria-pressed', 'true');
  fireEvent.click(screen.getByRole('button', { name: 'Se eligió mal al cobrar' }));
  expect(screen.getByLabelText('Motivo')).toHaveValue('Se eligió mal al cobrar');
  fireEvent.click(screen.getByRole('button', { name: 'Terminal B' }));
  fireEvent.click(screen.getByRole('button', { name: 'Confirmar' }));
  await waitFor(() => expect(api.changePaymentTerminal).toHaveBeenCalledWith(1179, 77,
    expect.objectContaining({ reason: 'Se eligió mal al cobrar' })));
});
