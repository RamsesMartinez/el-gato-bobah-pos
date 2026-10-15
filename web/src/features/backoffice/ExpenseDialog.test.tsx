import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ExpenseDialog } from './ExpenseDialog';

const units = [
  { id: 1, code: 'g', name: 'Gramo', kind: 'masa', toBase: '1' },
  { id: 2, code: 'kg', name: 'Kilogramo', kind: 'masa', toBase: '1000' },
  { id: 3, code: 'ml', name: 'Mililitro', kind: 'volumen', toBase: '1' },
  { id: 5, code: 'cda', name: 'Cucharada', kind: 'volumen', toBase: '15' },
  { id: 7, code: 'l', name: 'Litro', kind: 'volumen', toBase: '1000' },
  { id: 8, code: 'pieza', name: 'Pieza', kind: 'pieza', toBase: '1' },
  { id: 9, code: 'cm', name: 'Centímetro', kind: 'longitud', toBase: '1' },
];

const back = vi.hoisted(() => ({
  expenseCategories: vi.fn(),
  suppliers: vi.fn(),
  cashRegisters: vi.fn(),
  units: vi.fn(),
  presentations: vi.fn(),
  searchArticles: vi.fn(),
  createExpense: vi.fn(),
  createIngredient: vi.fn(),
  createExpenseCategory: vi.fn(),
  createSupplier: vi.fn(),
  parseDoc: vi.fn(),
  suggestArticles: vi.fn(),
  lastClosedCount: vi.fn(),
}));
const pos = vi.hoisted(() => ({
  paymentMethods: vi.fn(() => Promise.resolve({ items: [] as unknown[] })),
  cashStatus: vi.fn(() => Promise.resolve({ open: false })),
  businessSettings: vi.fn(() => Promise.resolve({ stockItemLabel: 'artículo', timezone: 'America/Mexico_City' })),
}));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
vi.mock('../../api/pos', () => ({ posApi: pos }));
// Doble del Picker sin hoja animada: el Picker real tiene sus pruebas, y en jsdom el cierre de su
// hoja deja oculto el diálogo durante un tiempo que no es estable. Aquí se prueba la captura.
vi.mock('../../components/Picker', async () => {
  const { useState } = await import('react');
  type Opt = { value: string; label: string };
  function Picker({ value, options, onChange, placeholder, onCreate }: {
    value: string; options: Opt[]; onChange: (v: string) => void; placeholder?: string;
    onCreate?: (name: string) => Promise<Opt | null>;
  }) {
    const [open, setOpen] = useState(false);
    const [q, setQ] = useState('');
    const selected = options.find((o) => o.value === value);
    return (
      <div>
        <button type="button" onClick={() => setOpen(true)}>{selected ? selected.label : placeholder}</button>
        {open && (
          <div>
            {onCreate && <input placeholder="Buscar…" value={q} onChange={(e) => setQ(e.target.value)} />}
            {options.map((o) => (
              <button key={o.value} type="button" onClick={() => { onChange(o.value); setOpen(false); }}>{o.label}</button>
            ))}
            {onCreate && q && (
              <button type="button" onClick={async () => { const o = await onCreate(q); if (o) onChange(o.value); setOpen(false); }}>
                Crear «{q}»
              </button>
            )}
          </div>
        )}
      </div>
    );
  }
  return { Picker };
});

beforeEach(() => {
  back.expenseCategories.mockResolvedValue({ items: [{ id: 4, name: 'Mercancía', financialGroup: 'operacional', isActive: true }] });
  back.suppliers.mockResolvedValue({ items: [{ id: 6, name: 'Lácteos del Centro', isActive: true }] });
  back.cashRegisters.mockResolvedValue({ items: [] });
  back.units.mockResolvedValue({ items: units });
  back.presentations.mockResolvedValue([
    { id: 2, ingredientId: 10, name: 'Caja', qtyInBase: '12000', baseUnitCode: 'ml', active: true, approximate: false, purchaseUnitId: null },
  ]);
  back.searchArticles.mockResolvedValue({ items: [
    { itemType: 'ingrediente', id: 10, name: 'Leche entera', unitCode: 'ml', unitKind: 'volumen' },
    { itemType: 'ingrediente', id: 11, name: 'Azúcar', unitCode: 'g', unitKind: 'masa' },
  ] });
  back.createExpense.mockResolvedValue({ id: 99 });
  back.createIngredient.mockResolvedValue({ id: 50, name: 'Canela' });
  back.lastClosedCount.mockResolvedValue({ closedAt: null });
  pos.businessSettings.mockResolvedValue({ stockItemLabel: 'artículo', timezone: 'America/Mexico_City' });
});
// Las hojas dejan en `body` el bloqueo de scroll y el `aria-hidden` de lo que tapan; sin limpiarlo,
// la prueba siguiente nace con su diálogo oculto.
afterEach(() => {
  cleanup();
  document.body.innerHTML = '';
  for (const a of ['style', 'data-inert', 'data-scroll-lock', 'aria-hidden']) document.body.removeAttribute(a);
  vi.clearAllMocks();
});

function montar() {
  const onSaved = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider>
    <ExpenseDialog open onClose={vi.fn()} onSaved={onSaved} />
  </Provider></QueryClientProvider>);
  return { onSaved };
}

const click = (name: string | RegExp) => fireEvent.click(screen.getByRole('button', { name }));

// Elige en un Picker: abre su hoja y toca la opción.
async function elegir(placeholder: string | RegExp, opcion: string | RegExp) {
  fireEvent.click(await screen.findByRole('button', { name: placeholder }));
  fireEvent.click(await screen.findByRole('button', { name: opcion }));
}

async function encabezado(total = '1140') {
  await elegir(/Elegir categoría/, 'Mercancía');
  fireEvent.change(screen.getByLabelText('Total de la compra'), { target: { value: total } });
}

async function capturaConMercancia() {
  montar();
  click(/Capturar sin ticket/);
  click('Sí');
  await encabezado();
}

describe('Captura de una compra (spec 038, maqueta C)', () => {
  test('abre por el ticket, con capturar a mano a un toque, y sin controles nativos', async () => {
    montar();
    expect(await screen.findByRole('button', { name: /Sube la foto del ticket/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Capturar sin ticket/ })).toBeInTheDocument();
    expect(document.querySelector('select')).toBeNull();
    expect(document.querySelector('input[type="date"]')).toBeNull();
  });

  test('un gasto sin mercancía no lleva renglones ni se marca recibido', async () => {
    montar();
    click(/Capturar sin ticket/);
    click('No, solo el gasto');
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
      await encabezado('640');
    click('Todavía no se paga');
    await waitFor(() => expect(screen.getByRole('button', { name: 'Guardar' })).toBeEnabled());
    fireEvent.click(await screen.findByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(back.createExpense).toHaveBeenCalled());
    const body = back.createExpense.mock.calls[0][0];
    expect(body.items).toEqual([]);
    expect(body.receivedAt).toBeUndefined();
  });

  test('sin categoría o sin total no se guarda', async () => {
    montar();
    click(/Capturar sin ticket/);
    click('No, solo el gasto');
    fireEvent.change(screen.getByLabelText('Total de la compra'), { target: { value: '640' } });
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
    expect(screen.getByTestId('pie-compra')).toHaveTextContent('falta la categoría');
  });

  test('un renglón en empaque dice cuánto suma, pide importe y manda el empaque', async () => {
    await capturaConMercancia();
    await elegir('Buscar o crear', /Leche entera/);
    // El empaque de la empresa llega elegido; las cucharadas no se ofrecen para comprar.
    expect(await screen.findByRole('button', { name: 'Caja · 12 L' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.queryByRole('button', { name: 'cda' })).toBeNull();
    fireEvent.change(screen.getByLabelText('Cantidad'), { target: { value: '3' } });
    expect(screen.getByText('= 36 L')).toBeInTheDocument();
    expect(screen.getByTestId('pie-compra')).toHaveTextContent('falta el importe de un renglón');
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
    fireEvent.change(screen.getByLabelText('Importe'), { target: { value: '540' } });
    expect(screen.getByTestId('pie-compra')).toHaveTextContent('el resto queda como «otros» del gasto');
    click('Todavía no se paga');
    click('Guardar');
    await waitFor(() => expect(back.createExpense).toHaveBeenCalled());
    const [item] = back.createExpense.mock.calls[0][0].items;
    expect(item).toMatchObject({ itemType: 'ingrediente', ingredientId: 10, presentationId: 2, quantity: '3', qtyReceived: '3', amount: '540' });
    expect(item.unitId).toBeUndefined();
  });

  test('«Llega después» se guarda sin recibir', async () => {
    await capturaConMercancia();
    await elegir('Buscar o crear', /Leche entera/);
    fireEvent.change(screen.getByLabelText('Importe'), { target: { value: '180' } });
    click('Llega después');
    click('Todavía no se paga');
    click('Guardar');
    await waitFor(() => expect(back.createExpense).toHaveBeenCalled());
    expect(back.createExpense.mock.calls[0][0].items[0].qtyReceived).toBe('0');
  });

  test('renglones que pasan del total no se guardan', async () => {
    await capturaConMercancia();
    await elegir('Buscar o crear', /Leche entera/);
    fireEvent.change(screen.getByLabelText('Importe'), { target: { value: '2000' } });
    expect(screen.getByTestId('pie-compra')).toHaveTextContent('los renglones suman $2,000');
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
  });

  test('un regalo va en 0 y se guarda', async () => {
    await capturaConMercancia();
    await elegir('Buscar o crear', /Leche entera/);
    click(/Regalo/);
    expect(screen.getByLabelText('Importe')).toHaveValue('0');
    click('Todavía no se paga');
    click('Guardar');
    await waitFor(() => expect(back.createExpense).toHaveBeenCalled());
    expect(back.createExpense.mock.calls[0][0].items[0]).toMatchObject({ isGift: true, amount: '0' });
  });

  test('crear un insumo pregunta en qué se mide y lo da de alta en su unidad base', async () => {
    await capturaConMercancia();
    fireEvent.click(await screen.findByRole('button', { name: 'Buscar o crear' }));
    fireEvent.change(await screen.findByPlaceholderText('Buscar…'), { target: { value: 'Canela' } });
    fireEvent.click(await screen.findByRole('button', { name: /Crear/ }));
    expect(await screen.findByText('¿En qué lo mides?')).toBeInTheDocument();
    for (const k of ['Peso (g, kg)', 'Líquido (ml, L)', 'Longitud (cm, m)', 'Piezas']) {
      expect(screen.getByText(k)).toBeInTheDocument();
    }
    fireEvent.click(screen.getByText('Peso (g, kg)'));
    await waitFor(() => expect(back.createIngredient).toHaveBeenCalledWith({ name: 'Canela', baseUnitId: 1 }));
  });

  test('quitar un renglón capturado pide confirmación', async () => {
    await capturaConMercancia();
    await elegir('Buscar o crear', /Leche entera/);
    click('Quitar renglón');
    expect(screen.getAllByTestId('renglon')).toHaveLength(1);
    click('Quitar');
    expect(screen.queryAllByTestId('renglon')).toHaveLength(0);
  });

  test('una compra con fecha anterior al último conteo avisa', async () => {
    // La fecha de la compra arranca hoy; un conteo «cerrado» después de hoy la deja antes.
    back.lastClosedCount.mockResolvedValue({ closedAt: '2099-01-01T18:00:00Z' });
    montar();
    click(/Capturar sin ticket/);
    click('Sí');
    expect(await screen.findByText(/Esta compra es de antes del último conteo/)).toBeInTheDocument();
  });

  test('una compra posterior al último conteo no avisa', async () => {
    back.lastClosedCount.mockResolvedValue({ closedAt: '2000-01-01T18:00:00Z' });
    montar();
    click(/Capturar sin ticket/);
    click('Sí');
    await screen.findByRole('button', { name: /Agregar artículo/ });
    await waitFor(() => expect(back.lastClosedCount).toHaveBeenCalled());
    expect(screen.queryByText(/de antes del último conteo/)).toBeNull();
  });

  test('la palabra del almacén es la de la empresa', async () => {
    pos.businessSettings.mockResolvedValue({ stockItemLabel: 'insumo', timezone: 'America/Mexico_City' });
    montar();
    click(/Capturar sin ticket/);
    click('Sí');
    expect(await screen.findByRole('button', { name: /Agregar insumo/ })).toBeInTheDocument();
  });
});

describe('desde el ticket', () => {
  beforeEach(() => {
    back.parseDoc.mockResolvedValue({
      doc: {
        issuedOn: '2026-10-14', total: '1140.00', kind: 'ticket', folio: 'A1', supplier: 'Lácteos',
        charges: [], payments: [], warnings: [],
        lines: [{ rawCode: '', rawName: 'AZUCAR STD 25K', qty: '1', amount: '600', unitPrice: '', packQty: '', packUnit: '', status: '' }],
      },
      raw: {}, reconciliation: { balanced: true, diff: '0', linesSum: '600', chargesSum: '0', breakdownSum: '0', total: '1140', hasSubtotal: false, linesMatchSubtotal: true, unreadable: [] },
    });
    back.suggestArticles.mockResolvedValue({ items: [
      { itemType: 'ingrediente', itemId: 11, itemName: 'Azúcar', source: 'parecido', score: 0.8, matchedVia: 'AZUCAR', packQtyInBase: '', unitId: null },
    ] });
  });

  async function subirTicket() {
    montar();
    const file = new File(['x'], 'ticket.jpg', { type: 'image/jpeg' });
    fireEvent.change(screen.getByTestId('ticket-file'), { target: { files: [file] } });
    return screen.findByTestId('renglon-dudoso');
  }

  test('un renglón dudoso va arriba, no suma y no bloquea guardar', async () => {
    const dudoso = await subirTicket();
    expect(dudoso).toHaveTextContent('Ticket: AZUCAR STD 25K');
    expect(dudoso).toHaveTextContent('¿Es Azúcar');
    expect(screen.getByTestId('pie-compra')).toHaveTextContent('1 renglón sin confirmar: no suma');
    await elegir(/Elegir categoría/, 'Mercancía');
    click('Todavía no se paga');
    await waitFor(() => expect(screen.getByRole('button', { name: 'Guardar' })).toBeEnabled());
    click('Guardar');
    await waitFor(() => expect(back.createExpense).toHaveBeenCalled());
    const [item] = back.createExpense.mock.calls[0][0].items;
    // Sin tocar el almacén ni enseñarle al proveedor que no es del almacén.
    expect(item).toMatchObject({ itemType: '', amount: '600' });
    expect(item.rawName).toBeUndefined();
  });

  test('«Sí, es ése» lo vuelve un renglón que suma', async () => {
    const dudoso = await subirTicket();
    fireEvent.click(within(dudoso).getByRole('button', { name: 'Sí, es ése' }));
    expect(screen.queryByTestId('renglon-dudoso')).toBeNull();
    expect(screen.getByTestId('pie-compra')).not.toHaveTextContent('sin confirmar');
  });

  test('un renglón surtido se parte en dos, y la mitad nueva no aprende el texto del ticket', async () => {
    const dudoso = await subirTicket();
    fireEvent.click(within(dudoso).getByRole('button', { name: 'Sí, es ése' }));
    await elegir(/Elegir categoría/, 'Mercancía');
    fireEvent.click(screen.getByRole('button', { name: /Partir/ }));
    expect(screen.getAllByTestId('renglon')).toHaveLength(2);
    const importes = screen.getAllByLabelText('Importe');
    fireEvent.change(importes[0], { target: { value: '400' } });
    await elegir('Buscar o crear', /Leche entera/);
    fireEvent.change(screen.getAllByLabelText('Importe')[1], { target: { value: '200' } });
    click('Todavía no se paga');
    click('Guardar');
    await waitFor(() => expect(back.createExpense).toHaveBeenCalled());
    const [a, b] = back.createExpense.mock.calls[0][0].items;
    expect(a).toMatchObject({ ingredientId: 11, rawName: 'AZUCAR STD 25K', amount: '400' });
    expect(b).toMatchObject({ ingredientId: 10, amount: '200' });
    expect(b.rawName).toBeUndefined();
  });

  test('si el total capturado no es el del ticket, pregunta cuál', async () => {
    await subirTicket();
    fireEvent.change(screen.getByLabelText('Total de la compra'), { target: { value: '1000' } });
    expect(screen.getByText(/El ticket dice \$1,140 y capturaste \$1,000/)).toBeInTheDocument();
    click(/el del ticket/);
    expect(screen.getByLabelText('Total de la compra')).toHaveValue('1140.00');
  });
});

// Revisión de tableta del 038: el pie juntaba lo que suma cada renglón en un solo texto, y un ticket
// de 10 renglones lo partía en 3 o 4 líneas que se comían el alto de la lista.
test('el pie dice lo que suman los primeros renglones y cuántos más', async () => {
  const lines = ['A', 'B', 'C', 'D', 'E'].map((n) => ({
    rawCode: '', rawName: `LECHE ${n}`, qty: '1', amount: '100', unitPrice: '', packQty: '', packUnit: '', status: '',
  }));
  back.parseDoc.mockResolvedValue({
    doc: { issuedOn: '2026-10-14', total: '500.00', kind: 'ticket', folio: '', supplier: '', charges: [], payments: [], warnings: [], lines },
    raw: {}, reconciliation: { balanced: true, diff: '0', linesSum: '500', chargesSum: '0', breakdownSum: '0', total: '500', hasSubtotal: false, linesMatchSubtotal: true, unreadable: [] },
  });
  back.suggestArticles.mockResolvedValue({ items: [
    { itemType: 'ingrediente', itemId: 10, itemName: 'Leche entera', source: 'aprendido', score: 1, matchedVia: 'LECHE', packQtyInBase: '', unitId: 7 },
  ] });
  montar();
  fireEvent.change(screen.getByTestId('ticket-file'), { target: { files: [new File(['x'], 't.jpg', { type: 'image/jpeg' })] } });
  await waitFor(() => expect(screen.getAllByTestId('renglon')).toHaveLength(5));
  const pie = screen.getByTestId('pie-compra');
  expect(pie).toHaveTextContent('y 2 más');
  expect(pie.textContent?.match(/Leche entera \+/g)).toHaveLength(3);
});

// Análisis del 038 (C1): T030 prometía estas tres y no estaban.
describe('lo que se crea desde la captura', () => {
  test('«+ Otro empaque» abre «Cómo llega» del insumo sin salir de la captura (FR-011)', async () => {
    await capturaConMercancia();
    await elegir('Buscar o crear', /Leche entera/);
    fireEvent.click(screen.getByRole('button', { name: '+ Otro empaque' }));
    expect(await screen.findByText('Cómo llega Leche entera')).toBeInTheDocument();
  });

  test('categoría y proveedor nuevos se crean con las rutas de la empresa (FR-004b)', async () => {
    back.createExpenseCategory.mockResolvedValue({ id: 40, name: 'Insumos de barra', financialGroup: 'operacional', isActive: true });
    back.createSupplier.mockResolvedValue({ id: 60, name: 'Abarrotes La Esquina', isActive: true });
    montar();
    click(/Capturar sin ticket/);
    click('No, solo el gasto');
    fireEvent.click(await screen.findByRole('button', { name: /Elegir categoría/ }));
    fireEvent.change(screen.getByPlaceholderText('Buscar…'), { target: { value: 'Insumos de barra' } });
    fireEvent.click(screen.getByRole('button', { name: /Crear «Insumos de barra»/ }));
    await waitFor(() => expect(back.createExpenseCategory).toHaveBeenCalledWith({ name: 'Insumos de barra', financialGroup: 'operacional' }));
    expect(await screen.findByRole('button', { name: 'Insumos de barra' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Sin proveedor/ }));
    fireEvent.change(screen.getByPlaceholderText('Buscar…'), { target: { value: 'Abarrotes La Esquina' } });
    fireEvent.click(screen.getByRole('button', { name: /Crear «Abarrotes La Esquina»/ }));
    await waitFor(() => expect(back.createSupplier).toHaveBeenCalledWith({ name: 'Abarrotes La Esquina' }));
  });

  test('el empaque que se elige gana sobre el tamaño aprendido del proveedor (regla 11)', async () => {
    back.parseDoc.mockResolvedValue({
      doc: { issuedOn: '2026-10-14', total: '540.00', kind: 'ticket', folio: '', supplier: '', charges: [], payments: [], warnings: [],
        lines: [{ rawCode: '', rawName: 'LECHE CAJA', qty: '3', amount: '540', unitPrice: '', packQty: '', packUnit: '', status: '' }] },
      raw: {}, reconciliation: { balanced: true, diff: '0', linesSum: '540', chargesSum: '0', breakdownSum: '0', total: '540', hasSubtotal: false, linesMatchSubtotal: true, unreadable: [] },
    });
    // Lo aprendido dice «pieza de 999 ml»; la empresa tiene «Caja · 12 L».
    back.suggestArticles.mockResolvedValue({ items: [
      { itemType: 'ingrediente', itemId: 10, itemName: 'Leche entera', source: 'aprendido', score: 1, matchedVia: 'LECHE', packQtyInBase: '999', unitId: 8 },
    ] });
    montar();
    fireEvent.change(screen.getByTestId('ticket-file'), { target: { files: [new File(['x'], 't.jpg', { type: 'image/jpeg' })] } });
    await screen.findByTestId('renglon');
    await elegir(/Elegir categoría/, 'Mercancía');
    fireEvent.click(await screen.findByRole('button', { name: 'Caja · 12 L' }));
    click('Todavía no se paga');
    click('Guardar');
    await waitFor(() => expect(back.createExpense).toHaveBeenCalled());
    const [item] = back.createExpense.mock.calls[0][0].items;
    expect(item).toMatchObject({ presentationId: 2 });
    expect(item.packQtyInBase).toBeUndefined();
    expect(item.unitId).toBeUndefined();
  });
});

// Prueba con usuarios sin contexto (T065): «Fecha» junto a «Día del gasto» se leía como dos fechas
// que no cuadran. Una es la del papel, la otra el día de la caja.
test('la fecha dice que es la del ticket', async () => {
  montar();
  click(/Capturar sin ticket/);
  click('No, solo el gasto');
  expect(await screen.findByText('Fecha del ticket')).toBeInTheDocument();
});

// Decisión del dueño tras la prueba sin contexto (FR-004d): «Total pagado» se leía como «ya está
// pagado», y sin un pago capturado aparte la compra quedaba como deuda.
describe('¿Cómo se pagó? (FR-004d)', () => {
  beforeEach(() => {
    pos.paymentMethods.mockResolvedValue({ items: [
      { id: 1, name: 'Efectivo', affectsCashDrawer: true, isActive: true },
      { id: 2, name: 'Tarjeta', affectsCashDrawer: false, isActive: true },
    ] });
  });

  async function gastoListo(total = '640') {
    montar();
    click(/Capturar sin ticket/);
    click('No, solo el gasto');
    await encabezado(total);
  }

  test('no se guarda sin contestar', async () => {
    await gastoListo();
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
    expect(screen.getByTestId('pie-compra')).toHaveTextContent('elige cómo se pagó');
  });

  test('un método registra un pago por el total y la compra queda pagada', async () => {
    await gastoListo();
    click('Tarjeta');
    click('Guardar');
    await waitFor(() => expect(back.createExpense).toHaveBeenCalled());
    const body = back.createExpense.mock.calls[0][0];
    expect(body.payments).toEqual([expect.objectContaining({ methodId: 2, amount: '640' })]);
    expect(body.status).toBe('pagada');
  });

  test('efectivo con una sola caja abierta la toma sola', async () => {
    back.cashRegisters.mockResolvedValue({ items: [{ id: 7, name: 'Caja 1', openSessionId: 70 }] });
    await gastoListo();
    click('Efectivo');
    click('Guardar');
    await waitFor(() => expect(back.createExpense).toHaveBeenCalled());
    expect(back.createExpense.mock.calls[0][0].payments).toEqual([expect.objectContaining({ methodId: 1, registerId: 7 })]);
  });

  test('efectivo con varias cajas abiertas pide de cuál sale', async () => {
    back.cashRegisters.mockResolvedValue({ items: [
      { id: 7, name: 'Caja 1', openSessionId: 70 }, { id: 8, name: 'Caja 2', openSessionId: 80 },
    ] });
    await gastoListo();
    click('Efectivo');
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
    await elegir(/¿De qué caja sale\?/, 'Caja 2');
    click('Guardar');
    await waitFor(() => expect(back.createExpense).toHaveBeenCalled());
    expect(back.createExpense.mock.calls[0][0].payments).toEqual([expect.objectContaining({ registerId: 8 })]);
  });

  test('sin caja abierta no se ofrece efectivo', async () => {
    await gastoListo();
    await screen.findByRole('button', { name: 'Tarjeta' });
    expect(screen.queryByRole('button', { name: 'Efectivo' })).toBeNull();
  });

  test('«Todavía no se paga» guarda sin pagos y queda pendiente', async () => {
    await gastoListo();
    click('Todavía no se paga');
    click('Guardar');
    await waitFor(() => expect(back.createExpense).toHaveBeenCalled());
    expect(back.createExpense.mock.calls[0][0]).toMatchObject({ payments: [], status: 'pendiente' });
  });

  test('con pagos capturados abajo no se pregunta', async () => {
    await gastoListo();
    click('Agregar pago');
    expect(screen.queryByRole('button', { name: 'Todavía no se paga' })).toBeNull();
  });
});

// Decisión del dueño tras la prueba sin contexto (FR-007e): «pedí 2, llegó 1» se dice en la captura.
describe('Llegó una parte (FR-007e)', () => {
  test('pide cuántos, acepta menos que lo pedido y manda esa cantidad', async () => {
    await capturaConMercancia();
    await elegir('Buscar o crear', /Leche entera/);
    fireEvent.change(screen.getByLabelText('Cantidad'), { target: { value: '3' } });
    fireEvent.change(screen.getByLabelText('Importe'), { target: { value: '540' } });
    click('Llegó una parte');
    const cuantos = screen.getByLabelText('¿Cuántos llegaron?');
    click('Todavía no se paga');
    for (const bad of ['', '0', '3', '4']) {
      fireEvent.change(cuantos, { target: { value: bad } });
      expect(screen.getByRole('button', { name: 'Guardar' }), `«${bad}» llegaron de 3`).toBeDisabled();
    }
    fireEvent.change(cuantos, { target: { value: '2' } });
    expect(screen.getByText('= 24 L')).toBeInTheDocument();
    click('Guardar');
    await waitFor(() => expect(back.createExpense).toHaveBeenCalled());
    expect(back.createExpense.mock.calls[0][0].items[0]).toMatchObject({ quantity: '3', qtyReceived: '2' });
  });
});
