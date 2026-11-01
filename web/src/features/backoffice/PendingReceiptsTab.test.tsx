import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import type { PendingReceipt } from '../../api/backoffice';
import { PendingReceiptsTab } from './PendingReceiptsTab';

const back = vi.hoisted(() => ({
  pendingReceipts: vi.fn(),
  receiveItem: vi.fn(),
  markNotComing: vi.fn(),
  receiveExpense: vi.fn(),
  addArrivedItem: vi.fn(),
  expenseDetail: vi.fn(),
  deliverExpense: vi.fn(),
  searchArticles: vi.fn(),
  presentations: vi.fn(),
  units: vi.fn(),
}));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
// Doble del Picker sin hoja animada (ver ExpenseDialog.test.tsx).
vi.mock('../../components/Picker', () => ({
  Picker: ({ options, onChange, placeholder }: { options: { value: string; label: string }[]; onChange: (v: string) => void; placeholder?: string }) => (
    <div>
      <span>{placeholder}</span>
      {options.map((o) => <button key={o.value} type="button" onClick={() => onChange(o.value)}>{o.label}</button>)}
    </div>
  ),
}));

const row = (over: Partial<PendingReceipt>): PendingReceipt => ({
  expenseId: 9, expenseDate: '2026-10-10', supplier: 'Lácteos del Centro', expenseAmount: '1140.00', paid: true,
  itemId: 41, itemName: 'Leche entera', quantity: '3', received: '0', notComing: '0', missing: '3', applied: '0',
  packQtyInBase: '12000', presentationName: 'Caja', unitCode: 'pieza', baseUnitCode: 'ml', ...over,
});

beforeEach(() => {
  back.pendingReceipts.mockResolvedValue([
    row({}),
    row({ itemId: 42, itemName: 'Crema para batir', quantity: '1', missing: '1', packQtyInBase: '1000', presentationName: 'Caja' }),
    row({ expenseId: 10, supplier: 'Abarrotes La Central', itemId: 50, itemName: 'Azúcar', quantity: '2', received: '1', missing: '1', applied: '25000',
      packQtyInBase: '25000', presentationName: 'Bulto', baseUnitCode: 'g', expenseAmount: '820.00' }),
  ]);
  for (const f of [back.receiveItem, back.markNotComing, back.receiveExpense, back.addArrivedItem]) f.mockResolvedValue(undefined);
  back.searchArticles.mockResolvedValue({ items: [{ itemType: 'ingrediente', id: 7, name: 'Yogurt natural', unitCode: 'ml', unitKind: 'volumen' }] });
  back.presentations.mockResolvedValue([{ id: 3, ingredientId: 7, name: 'Caja', qtyInBase: '12000', baseUnitCode: 'ml', active: true, approximate: false, purchaseUnitId: null }]);
  back.units.mockResolvedValue({ items: [{ id: 7, code: 'l', name: 'Litro', kind: 'volumen', toBase: '1000' }] });
});
afterEach(() => vi.clearAllMocks());

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><PendingReceiptsTab /></Provider></QueryClientProvider>);
}
const click = (name: string | RegExp) => fireEvent.click(screen.getByRole('button', { name }));
const renglon = async (name: string) => (await screen.findAllByTestId('renglon-por-recibir')).find((r) => r.textContent?.includes(name))!;

describe('Por recibir (spec 038, maqueta A con buscador)', () => {
  test('agrupa por compra y dice cuánto falta en su empaque', async () => {
    montar();
    const compras = await screen.findAllByTestId('compra-por-recibir');
    expect(compras).toHaveLength(2);
    expect(compras[0]).toHaveTextContent('Lácteos del Centro');
    expect(compras[0]).toHaveTextContent('compra del 10 oct · pagado $1,140');
    expect(await renglon('Azúcar')).toHaveTextContent('pediste 2 bultos de 25 kg · ya llegaron 1');
    expect(await renglon('Leche entera')).toHaveTextContent('3 cajas de 12 L');
  });

  test('el buscador filtra por artículo o proveedor', async () => {
    montar();
    await screen.findAllByTestId('compra-por-recibir');
    fireEvent.change(screen.getByLabelText('Buscar'), { target: { value: 'azucar' } });
    expect(screen.getAllByTestId('renglon-por-recibir')).toHaveLength(1);
    fireEvent.change(screen.getByLabelText('Buscar'), { target: { value: 'lácteos' } });
    expect(screen.getAllByTestId('renglon-por-recibir')).toHaveLength(2);
  });

  // Spec 045: la misma regla que el conteo; «ema» no es el inicio de ninguna palabra de «Crema».
  test('el buscador coincide al inicio de una palabra', async () => {
    montar();
    await screen.findAllByTestId('compra-por-recibir');
    fireEvent.change(screen.getByLabelText('Buscar'), { target: { value: 'ema' } });
    expect(screen.queryAllByTestId('renglon-por-recibir')).toHaveLength(0);
    fireEvent.change(screen.getByLabelText('Buscar'), { target: { value: 'crema bat' } });
    expect(screen.getAllByTestId('renglon-por-recibir')).toHaveLength(1);
  });

  test('«Ya no va a llegar…» pide cuántas, hasta lo que falta', async () => {
    montar();
    fireEvent.click(within(await renglon('Leche entera')).getByRole('button', { name: 'Ya no va a llegar…' }));
    const input = await screen.findByLabelText('Ya no llegan');
    fireEvent.change(input, { target: { value: '5' } });
    expect(screen.getByText('Solo faltan 3.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
    fireEvent.change(input, { target: { value: '1' } });
    click('Guardar');
    await waitFor(() => expect(back.markNotComing).toHaveBeenCalledWith(9, 41, '1'));
  });

  test('«Llegó algo más…»: lo pagué pide importe; fue regalo va en 0', async () => {
    montar();
    const [lacteos] = await screen.findAllByTestId('compra-por-recibir');
    fireEvent.click(within(lacteos).getByRole('button', { name: 'Llegó algo más…' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Yogurt natural' }));
    fireEvent.change(await screen.findByLabelText('Cantidad'), { target: { value: '2' } });
    expect(screen.getByRole('button', { name: 'Caja · 12 L' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
    click('Fue regalo');
    expect(screen.getByText(/No cambia lo pagado ni el costo/)).toBeInTheDocument();
    click('Guardar');
    await waitFor(() => expect(back.addArrivedItem).toHaveBeenCalledWith(9, expect.objectContaining({
      ingredientId: 7, quantity: '2', presentationId: 3, amount: '0', isGift: true,
    })));
  });

  test('sin nada por recibir lo dice, y sin controles nativos', async () => {
    back.pendingReceipts.mockResolvedValue([]);
    montar();
    expect(await screen.findByText('No hay nada por recibir.')).toBeInTheDocument();
    expect(document.querySelector('select')).toBeNull();
  });
});

// Spec 053: «Recibir» abre la hoja de entrega de la compra, con su fecha y su vista previa.
describe('Recibir desde «Por recibir» (spec 053)', () => {
  const item = (over: Record<string, unknown>) => ({
    itemType: 'ingrediente', ingredientId: 7, productId: null, description: 'x', unitCode: 'pieza', qtyReceived: null,
    unitCost: '1', amount: '100', packQtyInBase: '12000', applied: '0', baseUnitCode: 'ml', notComing: '0', isGift: false,
    presentationId: 2, deliveries: [], ...over,
  });
  beforeEach(() => {
    back.expenseDetail.mockResolvedValue({
      id: 9, expenseDate: '2026-10-10', receivedAt: null, status: 'pagada', category: 'Mercancía', financialGroup: 'operacional',
      supplier: 'Lácteos del Centro', amount: '1140', currency: 'MXN', description: null, docKind: null, docFolio: null,
      paymentMethod: null, paidAt: null, createdBy: null, itemCount: 2, payments: [], paid: '1140',
      items: [item({ id: 41, itemName: 'Leche entera', quantity: '3' }), item({ id: 42, itemName: 'Crema para batir', quantity: '1' })],
    });
    back.deliverExpense.mockResolvedValue({ preview: { header: [], lines: [], inventory: [], costs: [], decisions: [],
      money: { amountBefore: '1', amountAfter: '1', paid: '1', statusBefore: 'pagada', statusAfter: 'pagada', due: '0', overpaid: '0', decisionKey: null },
      fingerprint: 'f' }, expense: null });
  });

  test('«Recibir…» de un renglón precarga solo ése', async () => {
    montar();
    fireEvent.click(within(await renglon('Leche entera')).getByRole('button', { name: 'Recibir…' }));
    expect(await screen.findByLabelText('Cuántos llegaron de Leche entera')).toHaveValue('3');
    expect(screen.getByLabelText('Cuántos llegaron de Crema para batir')).toHaveValue('0');
    click('Ver efectos');
    await waitFor(() => expect(back.deliverExpense).toHaveBeenCalled());
    expect(back.deliverExpense.mock.calls[0][1].lines).toEqual([{ itemId: 41, qty: '3' }]);
  });

  test('«Recibir» de la compra precarga todo lo que falta', async () => {
    montar();
    const [lacteos] = await screen.findAllByTestId('compra-por-recibir');
    fireEvent.click(within(lacteos).getByRole('button', { name: 'Recibir' }));
    expect(await screen.findByLabelText('Cuántos llegaron de Crema para batir')).toHaveValue('1');
    expect(screen.getByText('2 de 2 renglones')).toBeInTheDocument();
  });

  // Prueba con usuarios sin contexto (T065 de la 038): la casilla del importe de lo de más no decía
  // que era dinero.
  test('llegó de más: pide el importe, con «$»', async () => {
    montar();
    fireEvent.click(within(await renglon('Leche entera')).getByRole('button', { name: 'Recibir…' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Más Leche entera' }));
    const extra = screen.getByLabelText('Importe de lo de más de Leche entera');
    expect(extra.parentElement).toHaveTextContent('$');
    expect(screen.getByRole('button', { name: 'Ver efectos' })).toBeDisabled();
  });
});
