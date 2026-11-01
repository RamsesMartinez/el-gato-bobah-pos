import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import type { ExpenseDetail, ExpenseItem } from '../../api/backoffice';
import { ExpenseDetailDialog } from './ExpenseDetailDialog';
import { useSessionStore } from '../../stores/session';
import userEvent from '@testing-library/user-event';

const back = vi.hoisted(() => ({
  expenseDetail: vi.fn(),
  units: vi.fn(),
  presentations: vi.fn(),
  saveStockLines: vi.fn(),
  receiveExpense: vi.fn(),
  cancelExpense: vi.fn(),
  searchArticles: vi.fn(() => Promise.resolve({ items: [] })),
  setItemExcluded: vi.fn(),
  setPaymentInCount: vi.fn(),
  editExpense: vi.fn(),
  deliverExpense: vi.fn(),
  expenseCategories: vi.fn(() => Promise.resolve({ items: [{ id: 4, name: 'Mercancía' }, { id: 5, name: 'Desechables' }] })),
  suppliers: vi.fn(() => Promise.resolve({ items: [{ id: 6, name: 'Lácteos del Centro' }] })),
}));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
vi.mock('../../api/pos', () => ({ posApi: {
  businessSettings: vi.fn(() => Promise.resolve({ stockItemLabel: 'artículo' })),
  paymentMethods: vi.fn(() => Promise.resolve({ items: [{ id: 1, name: 'Transferencia', affectsCashDrawer: false }] })),
} }));
// Doble del Picker sin hoja animada (ver ExpenseDialog.test.tsx).
vi.mock('../../components/Picker', () => ({
  Picker: ({ options, onChange, placeholder }: { options: { value: string; label: string }[]; onChange: (v: string) => void; placeholder?: string }) => (
    <div>
      <span>{placeholder}</span>
      {options.map((o) => <button key={o.value} type="button" onClick={() => onChange(o.value)}>{o.label}</button>)}
    </div>
  ),
}));

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

const sesion = (permissions: string[]) =>
  useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'admin', permissions } as never });

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider>
    <ExpenseDetailDialog id={9} onClose={vi.fn()} onChanged={vi.fn()} />
  </Provider></QueryClientProvider>);
}
const click = (name: string | RegExp) => fireEvent.click(screen.getByRole('button', { name }));

describe('Corregir lo que llegó (spec 038, maqueta B)', () => {
  beforeEach(() => sesion(['inventory.receive']));
  test('dice cómo llegó cada renglón y cuánto suma', async () => {
    montar();
    expect(await screen.findByText(/Llegaron 3 cajas \(12 L c\/u\)/)).toBeInTheDocument();
    expect(screen.getByText('Sumó 36 L al almacén')).toBeInTheDocument();
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
    expect(within(row).getByTestId('tendras')).toHaveTextContent('Tendrás 28 L (hoy 40 L)');
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
    expect(within(row).getByTestId('tendras')).toHaveTextContent('Tendrás 4 L (hoy 40 L)');
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

  // Spec 053: «Recibir» abre la hoja de entrega con lo que falta ya puesto (antes, «Llegó todo»
  // recibía sin vista previa).
  test('una compra que no ha llegado ofrece «Recibir», que abre la hoja de entrega', async () => {
    back.expenseDetail.mockResolvedValue(detail([leche({ qtyReceived: '0', applied: '0' })], { receivedAt: null }));
    montar();
    expect(await screen.findByText(/Se pidieron 3 cajas \(12 L c\/u\); no ha llegado/)).toBeInTheDocument();
    click(await screen.findByRole('button', { name: 'Recibir' }).then((b) => b.textContent!));
    expect(await screen.findByText('Recibir · Gasto 9')).toBeInTheDocument();
    expect(screen.getByLabelText('Cuántos llegaron de Leche entera')).toHaveValue('3');
  });
});

// Capturas del 038 en el ambiente de pruebas: «Tendrás 1 pieza guardados» y «Se pidieron 1 pieza».
test('una sola pieza se dice en singular', async () => {
  sesion(['inventory.receive']);
  back.expenseDetail.mockResolvedValue(detail([leche({ quantity: '1', qtyReceived: '0', applied: '0', presentationId: null,
    packQtyInBase: null, unitCode: 'pieza', baseUnitCode: 'pieza', onHand: '0' })], { receivedAt: null }));
  montar();
  expect(await screen.findByText(/Se pidió 1 pieza; no ha llegado/)).toBeInTheDocument();
  click(await screen.findByRole('button', { name: 'Corregir lo que llegó' }).then((b) => b.textContent!));
  fireEvent.click(within(await screen.findByTestId('renglon-recibido')).getByRole('button', { name: 'Llegó' }));
  expect(screen.getByTestId('tendras')).toHaveTextContent('Tendrás 1 pieza (hoy 0 piezas)');
});

describe('renglones excluidos (spec 049)', () => {
  const conPermiso = (permissions: string[]) =>
    useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'admin', permissions } as never });
  beforeEach(() => { conPermiso(['inventory.receive']); back.setItemExcluded.mockResolvedValue(undefined); });

  test('un excluido es una sola línea atenuada con su importe e «Incluir», sin pasos de recibir', async () => {
    back.expenseDetail.mockResolvedValue(detail([leche(), leche({ id: 42, itemName: 'Shampoo', amount: '80.00', excluded: true })]));
    montar();
    const fila = await screen.findByTestId('renglon-excluido');
    expect(fila).toHaveTextContent('Shampoo');
    expect(fila).toHaveTextContent('$80');
    expect(screen.getAllByTestId('renglon-recibido')).toHaveLength(1);
    fireEvent.click(within(fila).getByRole('button', { name: 'Incluir' }));
    await waitFor(() => expect(back.setItemExcluded).toHaveBeenCalledWith(9, 42, false));
  });

  test('uno incluido lleva «Excluir» con texto; si ya entró al almacén, pide confirmar (spec 050)', async () => {
    montar();
    const fila = await screen.findByTestId('renglon-recibido');
    const boton = within(fila).getByRole('button', { name: 'Excluir' });
    expect(boton).toHaveTextContent('Excluir');
    fireEvent.click(boton);
    expect(back.setItemExcluded).not.toHaveBeenCalled();
    expect(await screen.findByText('Sacar del almacén lo que llegó de Leche entera')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Excluir y sacar del almacén' }));
    await waitFor(() => expect(back.setItemExcluded).toHaveBeenCalledWith(9, 41, true));
  });

  test('si todavía no entró nada al almacén, «Excluir» no pregunta', async () => {
    back.expenseDetail.mockResolvedValue(detail([leche({ applied: '0', qtyReceived: null })]));
    montar();
    const fila = await screen.findByTestId('renglon-recibido');
    fireEvent.click(within(fila).getByRole('button', { name: 'Excluir' }));
    await waitFor(() => expect(back.setItemExcluded).toHaveBeenCalledWith(9, 41, true));
  });

  test('sin permiso de recibir no se ofrece excluir ni incluir', async () => {
    conPermiso([]);
    back.expenseDetail.mockResolvedValue(detail([leche(), leche({ id: 42, itemName: 'Shampoo', excluded: true })]));
    montar();
    await screen.findByTestId('renglon-excluido');
    expect(screen.queryByRole('button', { name: 'Excluir' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Incluir' })).toBeNull();
  });

  test('si rompe la regla del total, lo dice y no cambia', async () => {
    back.setItemExcluded.mockRejectedValue(new ApiError(400, 'VALIDATION', 'Los renglones pasan del total pagado', 'req-2'));
    back.expenseDetail.mockResolvedValue(detail([leche({ excluded: true })]));
    montar();
    fireEvent.click(within(await screen.findByTestId('renglon-excluido')).getByRole('button', { name: 'Incluir' }));
    expect(await screen.findByText(/pasan del total/)).toBeInTheDocument();
  });

  test('el total del ticket se ve solo si el documento lo trajo', async () => {
    back.expenseDetail.mockResolvedValue(detail([leche()], { documentTotal: '1220.00' }));
    montar();
    expect(await screen.findByText(/Total del ticket \$1,220/)).toBeInTheDocument();
  });

  test('sin total del documento no aparece', async () => {
    montar();
    await screen.findByTestId('renglon-recibido');
    expect(screen.queryByText(/Total del ticket/)).toBeNull();
  });
});

// Spec 048, R12: en el detalle, quien puede ajustar el arqueo cambia la casilla de un pago en efectivo.
describe('«Incluir en el arqueo» en el detalle del gasto', () => {
  const pago = (over: Record<string, unknown> = {}) => ({
    id: 31, methodId: 1, method: 'Efectivo', amount: '200.00', paidOn: '2026-10-12', inCashCount: true,
    reference: null, affectsCashDrawer: true, includeInCount: true, paidAt: '2026-10-12T15:00:00Z', sessionClosed: false, ...over,
  });
  const conPagos = (...payments: ReturnType<typeof pago>[]) =>
    back.expenseDetail.mockResolvedValue(detail([], { payments, status: 'pagada' } as never));
  const quien = (permissions: string[]) =>
    useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'gerente', permissions } as never });
  const interruptor = () => screen.queryByRole('checkbox', { name: /Incluir en el arqueo|Corte cerrado/ });

  test('con el permiso: interruptor encendido y apagarlo manda includeInCount false', async () => {
    quien(['cash.adjust_count']);
    conPagos(pago());
    back.setPaymentInCount.mockResolvedValue(undefined);
    montar();
    await screen.findByText('Efectivo');
    expect(interruptor()).toBeChecked();
    expect(interruptor()).toHaveAccessibleName('Incluir en el arqueo');
    await userEvent.click(interruptor()!);
    await waitFor(() => expect(back.setPaymentInCount).toHaveBeenCalledWith(9, 31, false));
  });

  test('un pago con diferencia dice cuánto salió de la caja (spec 050)', async () => {
    quien([]);
    conPagos(pago({ amount: '380.00', drawerExtra: '120.00' }));
    montar();
    expect(await screen.findByText('$380 · salieron $500 de la caja')).toBeInTheDocument();
  });

  test('corte cerrado: se ve «Corte cerrado» y no se puede cambiar', async () => {
    quien(['cash.adjust_count']);
    conPagos(pago({ sessionClosed: true }));
    montar();
    await screen.findByText('Efectivo');
    expect(interruptor()).toHaveAccessibleName('Corte cerrado');
    expect(interruptor()).toBeDisabled();
  });

  test('sin el permiso no hay interruptor', async () => {
    quien([]);
    conPagos(pago());
    montar();
    await screen.findByText('Efectivo');
    expect(interruptor()).toBeNull();
  });

  test('una tarjeta no lleva interruptor', async () => {
    quien(['cash.adjust_count']);
    conPagos(pago({ method: 'Tarjeta', affectsCashDrawer: false, includeInCount: false, inCashCount: false }));
    montar();
    await screen.findByText('Tarjeta');
    expect(interruptor()).toBeNull();
  });

  test('si el corte se cerró mientras tanto, lo dice y recarga', async () => {
    quien(['cash.adjust_count']);
    conPagos(pago({ includeInCount: false, inCashCount: false }));
    back.setPaymentInCount.mockRejectedValue(new ApiError(409, 'PAYMENT_IN_CLOSED_COUNT', 'cerrado', 'r1'));
    montar();
    await screen.findByText('Efectivo');
    const antes = back.expenseDetail.mock.calls.length;
    await userEvent.click(interruptor()!);
    await waitFor(() => expect(back.setPaymentInCount).toHaveBeenCalledWith(9, 31, true));
    await waitFor(() => expect(back.expenseDetail.mock.calls.length).toBeGreaterThan(antes));
  });
});

// Spec 053, maqueta «Detalle del gasto», opción A: ver y editar son dos estados.
describe('Editar un gasto guardado (spec 053)', () => {
  const preview = (over: Record<string, unknown> = {}) => ({
    header: [], lines: [], inventory: [], costs: [], decisions: [],
    money: { amountBefore: '67.50', amountAfter: '67.50', paid: '67.50', statusBefore: 'pagada', statusAfter: 'pagada', due: '0', overpaid: '0', decisionKey: null },
    fingerprint: 'f1', ...over,
  });
  beforeEach(() => {
    sesion(['expenses.edit', 'inventory.receive']);
    back.expenseDetail.mockResolvedValue(detail([], { amount: '67.50', paid: '67.50', status: 'pagada', supplierId: 6, categoryId: 4,
      payments: [{ id: 1, methodId: 1, method: 'Transferencia', amount: '67.50', paidOn: '2026-10-12', inCashCount: false,
        reference: null, affectsCashDrawer: false, paidAt: '2026-10-12T15:00:00Z', sessionClosed: false }] } as never));
    back.editExpense.mockResolvedValue({ preview: preview(), expense: null });
  });

  test('un gasto sin mercancía (el caso del 1-nov) ofrece «Editar gasto»', async () => {
    montar();
    expect(await screen.findByRole('button', { name: 'Editar gasto' })).toBeInTheDocument();
  });

  test('sin el permiso de editar no aparece «Editar gasto»', async () => {
    sesion(['inventory.receive']);
    montar();
    await screen.findByText(/Mercancía/);
    expect(screen.queryByRole('button', { name: 'Editar gasto' })).toBeNull();
  });

  test('quitar un renglón lo marca con «Deshacer» y lo manda al revisar', async () => {
    back.expenseDetail.mockResolvedValue(detail([leche({ qtyReceived: '0', applied: '0' })], { supplierId: 6, categoryId: 4 } as never));
    montar();
    click(await screen.findByRole('button', { name: 'Editar gasto' }).then((b) => b.textContent!));
    fireEvent.click(await screen.findByRole('button', { name: 'Quitar Leche entera' }));
    expect(screen.getByRole('button', { name: 'Deshacer' })).toBeInTheDocument();
    expect(screen.getByText('1 cambio sin confirmar')).toBeInTheDocument();
    click('Revisar cambios');
    await waitFor(() => expect(back.editExpense).toHaveBeenCalled());
    expect(back.editExpense.mock.calls[0][1]).toMatchObject({ remove: [41] });
    expect(back.editExpense.mock.calls[0][2]).toBe(true);
    click(await screen.findByRole('button', { name: 'Confirmar' }).then((b) => b.textContent!));
    await waitFor(() => expect(back.editExpense.mock.calls.at(-1)![2]).toBe(false));
    expect(back.editExpense.mock.calls.at(-1)![1]).toMatchObject({ remove: [41], fingerprint: 'f1' });
  });

  test('el encabezado se edita en línea y solo manda lo que cambió', async () => {
    montar();
    click(await screen.findByRole('button', { name: 'Editar gasto' }).then((b) => b.textContent!));
    fireEvent.click(await screen.findByRole('button', { name: 'Desechables' }));
    fireEvent.change(screen.getByLabelText('Comentario'), { target: { value: 'Pedido semanal' } });
    expect(screen.getByText('2 cambios sin confirmar')).toBeInTheDocument();
    click('Revisar cambios');
    await waitFor(() => expect(back.editExpense).toHaveBeenCalled());
    expect(back.editExpense.mock.calls[0][1].header).toEqual({ categoryId: 5, description: 'Pedido semanal' });
  });

  test('«Pagar al confirmar» en la vista previa abre el pago con el saldo puesto', async () => {
    back.editExpense.mockImplementation(async (_id: number, b: { decisions?: Record<string, string> }) => ({
      preview: preview({
        money: { amountBefore: '67.50', amountAfter: '80', paid: '67.50', statusBefore: 'pagada', statusAfter: 'pendiente', due: '12.50', overpaid: '0', decisionKey: 'money' },
        decisions: [{ key: 'money', kind: 'money', itemId: null, countId: null, countedOn: null, options: ['pay', 'leave'], chosen: b.decisions?.money ?? null }],
        fingerprint: JSON.stringify(b.decisions ?? {}),
      }),
      expense: null,
    }));
    back.expenseDetail.mockResolvedValue(detail([leche({ qtyReceived: '0', applied: '0' })], { supplierId: 6, categoryId: 4 } as never));
    montar();
    click(await screen.findByRole('button', { name: 'Editar gasto' }).then((b) => b.textContent!));
    fireEvent.click(await screen.findByRole('button', { name: 'Quitar Leche entera' }));
    click('Revisar cambios');
    fireEvent.click(await screen.findByRole('button', { name: 'Pagar al confirmar' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Confirmar' })).toBeEnabled());
    click('Confirmar');
    expect(await screen.findByDisplayValue('12.50')).toBeInTheDocument();
  });
});

// Revisión de tableta (spec 053): «Corregir lo que llegó» abría el teclado del sistema, que tapa el
// pie con «Guardar» en la tableta. Ahora usa el teclado en pantalla, como la hoja de entrega.
test('«Corregir lo que llegó» escribe la cantidad con el teclado en pantalla', async () => {
  sesion(['inventory.receive']);
  montar();
  click(await screen.findByRole('button', { name: 'Corregir lo que llegó' }).then((b) => b.textContent!));
  const row = await screen.findByTestId('renglon-recibido');
  const input = within(row).getByLabelText('Cuántos llegaron');
  expect(input).toHaveAttribute('inputmode', 'none');
  fireEvent.click(input);
  const pad = await screen.findByTestId('teclado');
  expect(screen.queryByRole('button', { name: 'Guardar' })).toBeNull(); // el teclado ocupa el pie
  fireEvent.click(within(pad).getByRole('button', { name: '2' }));
  expect(input).toHaveValue('2');
  expect(within(row).getByTestId('tendras')).toHaveTextContent('Tendrás 28 L (hoy 40 L)');
  fireEvent.click(screen.getByRole('button', { name: 'Listo' }));
  expect(screen.getByRole('button', { name: 'Guardar' })).toBeInTheDocument();
});

// Revisión de tableta del arreglo: con el teclado abierto, «No va a llegar» apagaba la casilla pero
// el teclado seguía escribiendo en ella.
test('cambiar el renglón a «No va a llegar» cierra el teclado de su casilla', async () => {
  sesion(['inventory.receive']);
  montar();
  click(await screen.findByRole('button', { name: 'Corregir lo que llegó' }).then((b) => b.textContent!));
  const row = await screen.findByTestId('renglon-recibido');
  fireEvent.click(within(row).getByLabelText('Cuántos llegaron'));
  await screen.findByTestId('teclado');
  fireEvent.click(within(row).getByRole('button', { name: 'No va a llegar' }));
  expect(screen.queryByTestId('teclado')).toBeNull();
  expect(screen.getByRole('button', { name: 'Guardar' })).toBeInTheDocument();
});
