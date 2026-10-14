import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import type { ExpenseDetail, ExpenseItem } from '../../api/backoffice';
import { ExpenseDetailDialog } from './ExpenseDetailDialog';

const back = vi.hoisted(() => ({
  expenseDetail: vi.fn(),
  units: vi.fn(),
  presentations: vi.fn(),
  saveStockLines: vi.fn(),
  receiveExpense: vi.fn(),
  cancelExpense: vi.fn(),
}));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

const leche = (over: Partial<ExpenseItem> = {}): ExpenseItem => ({
  id: 41, itemType: 'ingrediente', ingredientId: 7, productId: null, itemName: 'Leche entera', description: 'Leche',
  quantity: '3', unitCode: 'pieza', qtyReceived: '3', unitCost: '180', amount: '540.00', packQtyInBase: '12000',
  applied: '36000', baseUnitCode: 'ml', notComing: '0', isGift: false, presentationId: 2, onHand: '40000', ...over,
});
const detail = (items: ExpenseItem[], over: Partial<ExpenseDetail> = {}): ExpenseDetail => ({
  id: 9, expenseDate: '2026-10-12', documentDate: null, receivedAt: '2026-10-12', status: 'pendiente', category: 'Mercancía',
  financialGroup: 'operacional', supplier: 'Lácteos del Centro', amount: '1140.00', currency: 'MXN', description: null,
  docKind: null, docFolio: null, paymentMethod: null, paidAt: null, createdBy: 'Ana', itemCount: items.length,
  items, payments: [], paid: '0', ...over,
} as ExpenseDetail);

beforeEach(() => {
  back.expenseDetail.mockResolvedValue(detail([leche()]));
  back.units.mockResolvedValue({ items: [
    { id: 3, code: 'ml', name: 'Mililitro', kind: 'volumen', toBase: '1' },
    { id: 8, code: 'pieza', name: 'Pieza', kind: 'pieza', toBase: '1' },
  ] });
  back.presentations.mockResolvedValue([
    { id: 2, ingredientId: 7, name: 'Caja', qtyInBase: '12000', baseUnitCode: 'ml', active: true, approximate: false, purchaseUnitId: null },
  ]);
  back.saveStockLines.mockImplementation(() => Promise.resolve(detail([leche({ qtyReceived: '2', applied: '24000', onHand: '28000' })])));
  back.cancelExpense.mockResolvedValue(undefined);
  back.receiveExpense.mockResolvedValue(undefined);
});
afterEach(() => vi.clearAllMocks());

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider>
    <ExpenseDetailDialog id={9} onClose={vi.fn()} onChanged={vi.fn()} />
  </Provider></QueryClientProvider>);
}
const click = (name: string | RegExp) => fireEvent.click(screen.getByRole('button', { name }));

describe('Corregir lo que llegó (spec 038, maqueta B)', () => {
  test('dice cómo llegó cada renglón y cuánto suma', async () => {
    montar();
    expect(await screen.findByText(/Llegaron 3 cajas \(12 L c\/u\)/)).toBeInTheDocument();
    expect(screen.getByText('36 L en el almacén')).toBeInTheDocument();
  });

  test('tres estados por renglón, «Tendrás X (hoy Y)» y el resumen de la diferencia', async () => {
    montar();
    click(await screen.findByRole('button', { name: 'Corregir lo que llegó' }).then((b) => b.textContent!));
    const row = await screen.findByTestId('renglon-recibido');
    for (const s of ['Llegó', 'Llega después', 'No va a llegar']) expect(within(row).getByRole('button', { name: s })).toBeInTheDocument();
    expect(within(row).getByRole('button', { name: 'Llegó' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByTestId('resumen-correccion')).toHaveTextContent('Sin cambios todavía');
    fireEvent.click(within(row).getByRole('button', { name: 'Menos' }));
    // 2 cajas = 24 L: hoy 40 L, la compra había sumado 36 → quedarán 28 L.
    expect(within(row).getByTestId('tendras')).toHaveTextContent('Tendrás 28 L guardados (hoy 40 L)');
    expect(screen.getByTestId('resumen-correccion')).toHaveTextContent('Leche entera: −12 L');
    // La cifra se vuelve a pedir al servidor antes de habilitar Guardar.
    await waitFor(() => expect(back.expenseDetail.mock.calls.length).toBeGreaterThanOrEqual(3));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Guardar' })).toBeEnabled());
    click('Guardar');
    await waitFor(() => expect(back.saveStockLines).toHaveBeenCalled());
    const [, lines] = back.saveStockLines.mock.calls[0];
    expect(lines).toEqual([expect.objectContaining({ id: 41, seenApplied: '36000', quantity: '3', received: '2', notComing: '0', presentationId: 2, amount: '540.00' })]);
  });

  test('«No va a llegar» cierra lo pedido y no suma', async () => {
    montar();
    click(await screen.findByRole('button', { name: 'Corregir lo que llegó' }).then((b) => b.textContent!));
    const row = await screen.findByTestId('renglon-recibido');
    fireEvent.click(within(row).getByRole('button', { name: 'No va a llegar' }));
    expect(within(row).getByTestId('tendras')).toHaveTextContent('Tendrás 4 L guardados (hoy 40 L)');
    await waitFor(() => expect(screen.getByRole('button', { name: 'Guardar' })).toBeEnabled());
    click('Guardar');
    await waitFor(() => expect(back.saveStockLines).toHaveBeenCalled());
    expect(back.saveStockLines.mock.calls[0][1][0]).toMatchObject({ received: '0', notComing: '3' });
  });

  test('si otra tableta corrigió primero, recarga y lo dice', async () => {
    back.saveStockLines.mockRejectedValue(new ApiError(409, 'CONFLICT', 'conflicto', 'req-1'));
    montar();
    click(await screen.findByRole('button', { name: 'Corregir lo que llegó' }).then((b) => b.textContent!));
    fireEvent.click(within(await screen.findByTestId('renglon-recibido')).getByRole('button', { name: 'Menos' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Guardar' })).toBeEnabled());
    click('Guardar');
    expect(await screen.findByText(/Alguien más corrigió esta compra/)).toBeInTheDocument();
    expect(screen.getByTestId('resumen-correccion')).toHaveTextContent('Sin cambios todavía');
  });

  test('anular confirma diciendo cuánto quedará, y después se ve anulada', async () => {
    montar();
    click(await screen.findByRole('button', { name: 'Anular compra' }).then((b) => b.textContent!));
    expect(screen.getByTestId('confirmar-anular')).toHaveTextContent('Leche entera: quedarán 4 L guardados');
    expect(back.cancelExpense).not.toHaveBeenCalled();
    back.expenseDetail.mockResolvedValue(detail([leche({ applied: '0', onHand: '4000' })], { status: 'cancelada' }));
    click('Sí, anular');
    await waitFor(() => expect(back.cancelExpense).toHaveBeenCalledWith(9, ''));
    expect(await screen.findByText('Anulada')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Corregir lo que llegó' })).toBeNull();
  });

  test('una compra que no ha llegado ofrece «Llegó todo»', async () => {
    back.expenseDetail.mockResolvedValue(detail([leche({ qtyReceived: '0', applied: '0' })], { receivedAt: null }));
    montar();
    expect(await screen.findByText(/Se pidieron 3 cajas \(12 L c\/u\); no ha llegado/)).toBeInTheDocument();
    click(await screen.findByRole('button', { name: 'Llegó todo' }).then((b) => b.textContent!));
    await waitFor(() => expect(back.receiveExpense).toHaveBeenCalledWith(9, {}));
  });
});
