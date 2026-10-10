import { vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';
import type { BoardOrder, OrderLine, OrderView, PaymentView } from '../types/pos';
import { useSessionStore } from '../stores/session';
import { ApiError } from '../api/client';

const order = vi.hoisted(() => vi.fn());
const paymentMethods = vi.hoisted(() => vi.fn());
const chargeOrder = vi.hoisted(() => vi.fn());
const chargeOrderShape = vi.hoisted(() => vi.fn());
const quoteOrder = vi.hoisted(() => vi.fn());
const voidPayment = vi.hoisted(() => vi.fn());
const moveLines = vi.hoisted(() => vi.fn());
const activeOrders = vi.hoisted(() => vi.fn());
const setOrderDiscount = vi.hoisted(() => vi.fn());
const medirAccion = vi.hoisted(() => vi.fn());
vi.mock('../api/pos', () => ({
  posApi: {
    order, paymentMethods, chargeOrder, chargeOrderShape, quoteOrder, voidPayment, moveLines, activeOrders,
    setOrderDiscount,
    cardTerminals: () => Promise.resolve({ items: [{ id: 5, branchId: 1, branchName: 'Matriz', name: 'Getnet', archived: false }] }),
    defaultTerminal: () => Promise.resolve(null), setDefaultTerminal: () => Promise.resolve(),
    businessSettings: () => Promise.resolve({ timezone: 'America/Mexico_City' }),
  },
}));
vi.mock('../api/uso', () => ({ medirAccion }));

import { CobrarSheet } from './CobrarSheet';

function pinta(nodo: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <ChakraProvider value={defaultSystem}>
      <QueryClientProvider client={qc}>{nodo}</QueryClientProvider>
    </ChakraProvider>,
  );
}

const line = (id: number, name: string, quantity: string, lineTotal: string, over: Partial<OrderLine> = {}): OrderLine => ({
  id, productName: name, quantity, unitPrice: String(Number(lineTotal) / Number(quantity)), lineTotal,
  delivered: '0', cancelled: false, ...over,
});
const pay = (number: number, method: string, amount: string, lines: Array<[number, string]>, over: Partial<PaymentView> = {}): PaymentView => ({
  id: number * 10, number, voided: false, methodId: method === 'Efectivo' ? 1 : 2, methodName: method, amount, tip: '0',
  reference: '', paidAt: '2026-10-07T21:23:28Z', receivedBy: 'carlos', split: null,
  lines: lines.map(([lineId, qty]) => ({ lineId, qty, amount })), ...over,
});

const LINES = [
  line(1, 'Soju Original', '1', '110'),
  line(2, 'Capuccino', '2', '126'),
  line(3, 'Ramune Original', '1', '75'),
];
const board = (over: Partial<BoardOrder> = {}): BoardOrder => ({
  id: 7, number: 3, folioName: 'Persa', status: 'abierta', serviceType: 'mostrador', deliveryPlatformId: null,
  customerName: null, total: '311', currency: 'MXN', paid: false, outstanding: '311',
  openedAt: new Date().toISOString(), enPreparacion: true, renglones: 3, lines: [], ...over,
});
const view = (over: Partial<OrderView> = {}): OrderView => ({
  id: 7, number: 3, folioName: 'Persa', status: 'abierta', serviceType: 'mostrador', deliveryPlatformId: null,
  platformOrderRef: null, customerName: null, subtotal: '311', discount: '0', deliveryFee: '0', total: '311',
  currency: 'MXN', paid: false, outstanding: '311', openedAt: new Date().toISOString(), lines: LINES,
  payments: [], mergedIntoOrderId: null, canSplit: true, ...over,
});

const metodos = [
  { id: 1, name: 'Efectivo', kind: 'efectivo', deliveryPlatformId: null },
  { id: 2, name: 'Tarjeta', kind: 'tarjeta', deliveryPlatformId: null },
];

function signIn(permissions: string[]) {
  useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'gerente', permissions } as never });
}

beforeEach(() => {
  vi.clearAllMocks();
  order.mockResolvedValue(view());
  paymentMethods.mockResolvedValue({ items: metodos });
  quoteOrder.mockImplementation(async (_id: number, shape: { lines?: Array<{ lineId: number; qty: string }>; allRemaining?: boolean }) => {
    if (shape.allRemaining) return { amount: '311', lines: [], outstandingAfter: '0' };
    const amount = (shape.lines ?? []).reduce((n, l) => {
      const ln = LINES.find((x) => x.id === l.lineId)!;
      return n + (Number(ln.lineTotal) / Number(ln.quantity)) * Number(l.qty);
    }, 0);
    return { amount: String(amount), lines: [], outstandingAfter: String(311 - amount) };
  });
  activeOrders.mockResolvedValue({ items: [] });
  signIn(['orders.move_lines', 'orders.cancel_pending']);
});

async function openByProducts(u: ReturnType<typeof userEvent.setup>) {
  await u.click(await screen.findByRole('button', { name: /Dividir/ }));
  // Con productos por pagar, «Dividir» abre por productos.
  expect(screen.getByRole('button', { name: 'Por productos', pressed: true })).toBeInTheDocument();
}

// EL SELECTOR SOLO APARECE AL TOCAR «DIVIDIR»: quien cobra a una sola persona no paga su alto.
test('el selector de modo solo aparece tras tocar Dividir', async () => {
  const u = userEvent.setup();
  pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
  expect(await screen.findByRole('button', { name: /^Cobrar \$311/ })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Por productos' })).toBeNull();
  await openByProducts(u);
  expect(screen.getByRole('button', { name: 'Entre personas' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Por monto' })).toBeInTheDocument();
});

// POR PRODUCTOS: casillas grandes, el contador «1 de 2» y el monto que cotiza el servidor.
test('por productos, la selección se cotiza y el botón cobra lo que dice el servidor', async () => {
  const u = userEvent.setup();
  chargeOrderShape.mockResolvedValue({ outstanding: '138', paid: false, yaEstaba: false, amount: '173' });
  pinta(<CobrarSheet pantalla="pedidos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
  await openByProducts(u);

  const soju = screen.getByRole('button', { name: 'Soju Original' });
  expect(parseInt(getComputedStyle(soju).minHeight, 10)).toBeGreaterThanOrEqual(48);
  await u.click(soju);
  await u.click(screen.getByRole('button', { name: /Capuccino/ }));
  // Un renglón de dos arranca en una pieza, con − y + de 44 px.
  expect(screen.getByText('1 de 2')).toBeInTheDocument();
  const more = screen.getByLabelText('Una Capuccino más');
  expect(parseInt(getComputedStyle(more).minHeight, 10)).toBeGreaterThanOrEqual(44);

  expect(await screen.findByRole('button', { name: /^Cobrar \$173/ })).toBeInTheDocument();
  // Una ráfaga de toques es una sola cotización, no una por toque.
  await waitFor(() => expect(quoteOrder).toHaveBeenCalledTimes(1));
  expect(quoteOrder).toHaveBeenCalledWith(7, { lines: [{ lineId: 1, qty: '1' }, { lineId: 2, qty: '1' }] });

  await u.click(screen.getByRole('button', { name: 'Tarjeta' }));
  await u.click(screen.getByRole('button', { name: /^Cobrar \$173/ }));
  await waitFor(() => expect(chargeOrderShape).toHaveBeenCalledTimes(1));
  const [, body] = chargeOrderShape.mock.calls[0];
  expect(body.lines).toEqual([{ lineId: 1, qty: '1' }, { lineId: 2, qty: '1' }]);
  expect(body).not.toHaveProperty('amount');
  expect(medirAccion).toHaveBeenCalledWith('pedidos', 'split-by-products');
});

// «TODO LO QUE FALTA» MANDA allRemaining, no la lista que la hoja tenía: puede estar vieja.
test('Todo lo que falta manda allRemaining', async () => {
  const u = userEvent.setup();
  chargeOrderShape.mockResolvedValue({ outstanding: '0', paid: true, yaEstaba: false, amount: '311' });
  pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
  await openByProducts(u);
  await u.click(screen.getByRole('button', { name: 'Todo lo que falta' }));
  await u.click(screen.getByRole('button', { name: 'Efectivo' }));
  await u.click(await screen.findByRole('button', { name: /^Cobrar \$311/ }));
  await waitFor(() => expect(chargeOrderShape).toHaveBeenCalledTimes(1));
  expect(chargeOrderShape.mock.calls[0][1]).toMatchObject({ allRemaining: true });
  expect(chargeOrderShape.mock.calls[0][1]).not.toHaveProperty('lines');
});

// LO PAGADO SIGUE A LA VISTA TRAS RECARGAR: viene de los pagos del servidor, en gris y con su pago.
test('lo pagado sale en gris con su pago, y las fichas dicen número, método y monto', async () => {
  const u = userEvent.setup();
  order.mockResolvedValue(view({ outstanding: '201', payments: [pay(1, 'Tarjeta', '110', [[1, '1']])] }));
  pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
  expect(await screen.findByText(/Pago 1 · Tarjeta · \$110/)).toBeInTheDocument();
  await openByProducts(u);
  // El pagado ya no es una casilla: es un renglón gris con «Pago 1 · Tarjeta».
  expect(screen.queryByRole('button', { name: 'Soju Original' })).toBeNull();
  expect(screen.getByText('Pago 1 · Tarjeta')).toBeInTheDocument();
});

// ANTE «ESE PRODUCTO YA SE PAGÓ», LA HOJA RELEE EL PEDIDO: otra tableta cobró esa pieza.
test('si otra tableta ya cobró la pieza, la hoja relee el pedido y suelta la selección', async () => {
  const u = userEvent.setup();
  chargeOrderShape.mockRejectedValue(new ApiError(409, 'CONFLICT', 'Ese producto ya se pagó', 'r1'));
  pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
  await openByProducts(u);
  await u.click(screen.getByRole('button', { name: 'Soju Original' }));
  await u.click(screen.getByRole('button', { name: 'Tarjeta' }));
  const reads = order.mock.calls.length;
  await u.click(await screen.findByRole('button', { name: /^Cobrar \$110/ }));
  expect(await screen.findAllByText('Ese producto ya se pagó')).not.toHaveLength(0);
  await waitFor(() => expect(order.mock.calls.length).toBeGreaterThan(reads));
  expect(screen.getByRole('button', { name: 'Soju Original' })).toHaveAttribute('aria-pressed', 'false');
});

// CON PAGOS, EL DESCUENTO YA NO SE OFRECE: el servidor lo rechaza (D-17).
test('con un pago hecho no se ofrece Descuento', async () => {
  order.mockResolvedValue(view({ outstanding: '201', payments: [pay(1, 'Tarjeta', '110', [[1, '1']])] }));
  pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
  expect(await screen.findByText(/Pago 1 · Tarjeta · \$110/)).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /Descuento/ })).toBeNull();
});

// UN PEDIDO QUE NO SE DIVIDE NO OFRECE DIVIDIRLO POR PRODUCTOS NI ENTRE PERSONAS.
test('en un pedido de plataforma o de un turno cerrado solo queda Por monto', async () => {
  const u = userEvent.setup();
  order.mockResolvedValue(view({ canSplit: false }));
  pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
  await u.click(await screen.findByRole('button', { name: /Dividir/ }));
  expect(screen.getByRole('button', { name: 'Por monto' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Por productos' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Entre personas' })).toBeNull();
});

// POR MONTO, CON TECLADO PROPIO: el del sistema se come media pantalla.
test('por monto teclea con su propio teclado de dos filas y cobra lo tecleado', async () => {
  const u = userEvent.setup();
  chargeOrder.mockResolvedValue({ outstanding: '61', paid: false, yaEstaba: false, amount: '250' });
  pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
  await u.click(await screen.findByRole('button', { name: /Dividir/ }));
  await u.click(screen.getByRole('button', { name: 'Por monto' }));
  for (const k of ['2', '5', '0']) await u.click(screen.getByRole('button', { name: k }));
  expect(parseInt(getComputedStyle(screen.getByRole('button', { name: '5' })).minHeight, 10)).toBeGreaterThanOrEqual(48);
  expect(screen.getByRole('button', { name: /Lo que falta · \$311/ })).toBeInTheDocument();
  await u.click(screen.getByRole('button', { name: 'Efectivo' }));
  await u.click(screen.getByRole('button', { name: /^Cobrar \$250/ }));
  await waitFor(() => expect(chargeOrder).toHaveBeenCalledTimes(1));
  expect(chargeOrder.mock.calls[0][1].amount).toBe(250);
});

// LA SELECCIÓN QUE NO CABE TRAS UN PAGO POR MONTO DICE QUÉ HACER.
test('si la selección pasa de lo que falta, la hoja lo dice con el texto del servidor', async () => {
  const u = userEvent.setup();
  quoteOrder.mockRejectedValue(new ApiError(400, 'VALIDATION',
    'Ya se cobraron $250.00 sin elegir productos. Esta selección pasa de lo que falta: usa «Todo lo que falta»', 'r2'));
  pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
  await openByProducts(u);
  await u.click(screen.getByRole('button', { name: 'Soju Original' }));
  expect(await screen.findByText(/Esta selección pasa de lo que falta/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /^Cobrar/ })).toBeDisabled();
});

// DEVOLVER UN PAGO: desde su ficha, con permiso, motivo sin preselección y aviso según el método.
test('la ficha abre el detalle y devolver pide permiso y motivo', async () => {
  const u = userEvent.setup();
  order.mockResolvedValue(view({ outstanding: '201', payments: [pay(1, 'Tarjeta', '110', [[1, '1']])] }));
  signIn([]);
  const { unmount } = pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
  await u.click(await screen.findByRole('button', { name: 'Pago 1' }));
  expect(screen.getByText('Cubrió 1 producto')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /Devolver este pago/ })).toBeDisabled();
  expect(screen.getByText('Pídele a quien encargue la caja que lo devuelva')).toBeInTheDocument();
  unmount();

  signIn(['payments.void']);
  voidPayment.mockResolvedValue({ outstanding: '311', paid: false });
  pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
  await u.click(await screen.findByRole('button', { name: 'Pago 1' }));
  await u.click(screen.getByRole('button', { name: /Devolver este pago/ }));
  const confirm = screen.getByRole('button', { name: /^Devolver \$110/ });
  // Ningún motivo viene elegido: quedaría en la bitácora sin que nadie lo eligiera.
  expect(confirm).toBeDisabled();
  expect(screen.getByText(/El reembolso en la terminal se hace aparte/)).toBeInTheDocument();
  await u.click(screen.getByRole('button', { name: 'Se le cobró a otra persona' }));
  await u.click(confirm);
  await waitFor(() => expect(voidPayment).toHaveBeenCalledWith(7, 10, 'Se le cobró a otra persona'));
  expect(medirAccion).toHaveBeenCalledWith('pos', 'void-payment');
});

test('un pago devuelto sale tachado con su número', async () => {
  order.mockResolvedValue(view({ payments: [pay(1, 'Tarjeta', '110', [], { voided: true, voidReason: 'Se cobró de más' })] }));
  pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
  const chip = await screen.findByRole('button', { name: 'Pago 1, devuelto' });
  expect(within(chip).getByText('Devuelto')).toBeInTheDocument();
});

// PASAR A OTRO PEDIDO.
describe('pasar a otro pedido', () => {
  const others = [board({ id: 8, number: 6, folioName: 'Singapura', renglones: 2, total: '93' })];

  test('con descuento, «Pasar» se apaga con su motivo antes de abrir la lista', async () => {
    const u = userEvent.setup();
    order.mockResolvedValue(view({ discount: '10' }));
    pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
    await openByProducts(u);
    await u.click(screen.getByRole('button', { name: 'Soju Original' }));
    expect(screen.getByRole('button', { name: /Pasar a otro pedido/ })).toBeDisabled();
    expect(screen.getByText('Quita el descuento antes de pasar productos')).toBeInTheDocument();
  });

  test('con todo elegido, «Pedido nuevo» se apaga con «Ya es su propio pedido»', async () => {
    const u = userEvent.setup();
    activeOrders.mockResolvedValue({ items: others });
    pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
    await openByProducts(u);
    await u.click(screen.getByRole('button', { name: 'Todo lo que falta' }));
    await u.click(screen.getByRole('button', { name: /Pasar a otro pedido/ }));
    expect(await screen.findByRole('button', { name: /Pedido nuevo/ })).toBeDisabled();
    expect(screen.getByText('Ya es su propio pedido; no hace falta pasarlo')).toBeInTheDocument();
  });

  test('tocar un destino lo marca y pide confirmar; el buscador no se enfoca solo', async () => {
    const u = userEvent.setup();
    activeOrders.mockResolvedValue({ items: others });
    moveLines.mockResolvedValue({ from: view({ total: '201' }), to: view({ id: 8, number: 6, folioName: 'Singapura' }) });
    pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
    await openByProducts(u);
    await u.click(screen.getByRole('button', { name: 'Soju Original' }));
    await u.click(screen.getByRole('button', { name: /Pasar a otro pedido/ }));
    const search = await screen.findByLabelText('Buscar pedido');
    expect(search).not.toHaveFocus();
    const row = await screen.findByRole('button', { name: /Singapura/ });
    expect(parseInt(getComputedStyle(row).minHeight, 10)).toBeGreaterThanOrEqual(56);
    await u.click(row);
    expect(moveLines).not.toHaveBeenCalled();
    await u.click(screen.getByRole('button', { name: 'Pasar a Singapura' }));
    await waitFor(() => expect(moveLines).toHaveBeenCalledTimes(1));
    expect(moveLines.mock.calls[0][1]).toMatchObject({ toOrderId: 8, lines: [{ lineId: 1, qty: '1' }] });
    expect(medirAccion).toHaveBeenCalledWith('pos', 'move-lines');
  });

  test('tras pasar a un pedido nuevo, la hoja ofrece cobrarlo', async () => {
    const u = userEvent.setup();
    moveLines.mockResolvedValue({ from: view({ total: '201' }), to: view({ id: 9, number: 7, folioName: 'Aegean', total: '110', outstanding: '110' }) });
    pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
    await openByProducts(u);
    await u.click(screen.getByRole('button', { name: 'Soju Original' }));
    await u.click(screen.getByRole('button', { name: /Pasar a otro pedido/ }));
    await u.click(await screen.findByRole('button', { name: /Pedido nuevo/ }));
    await u.click(screen.getByRole('button', { name: 'Pasar a un pedido nuevo' }));
    expect(await screen.findByRole('button', { name: 'Cobrar #7' })).toBeInTheDocument();
    expect(moveLines.mock.calls[0][1].toOrderId).toBeNull();
  });

  test('si el origen se juntó con el destino, la hoja se cierra', async () => {
    const u = userEvent.setup();
    const onClose = vi.fn();
    activeOrders.mockResolvedValue({ items: others });
    moveLines.mockResolvedValue({ from: view({ status: 'cancelada', mergedIntoOrderId: 8 }), to: view({ id: 8 }) });
    pinta(<CobrarSheet pantalla="pos" order={board()} onClose={onClose} onCobrado={() => {}} />);
    await openByProducts(u);
    await u.click(screen.getByRole('button', { name: 'Soju Original' }));
    await u.click(screen.getByRole('button', { name: /Pasar a otro pedido/ }));
    await u.click(await screen.findByRole('button', { name: /Singapura/ }));
    await u.click(screen.getByRole('button', { name: 'Pasar a Singapura' }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });
});

// A 600 PX NADA SE ESCONDE DETRÁS DEL PIE (validación como usuario nuevo). Por productos y con
// efectivo, el cuerpo y el pie tenían cada uno su scroll: los billetes y los métodos quedaban
// cortados sin que se viera que había más. El pie es UNA fila —métodos y «Cobrar», como en el
// lienzo— y lo de elegir productos sube al cuerpo.
describe('la hoja de cobro cabe en la tableta', () => {
  test('métodos y Cobrar van en una sola fila; el pie no tiene scroll propio', async () => {
    const u = userEvent.setup();
    pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
    await openByProducts(u);
    await u.click(screen.getByRole('button', { name: 'Soju Original' }));
    await u.click(screen.getByRole('button', { name: 'Efectivo' }));
    const pie = screen.getByTestId('pie-de-cobro');
    expect(getComputedStyle(pie).overflowY).not.toBe('auto');
    const metodos = within(pie).getByRole('group', { name: '¿Con qué paga?' });
    const cobrar = within(pie).getByRole('button', { name: /^Cobrar \$/ });
    expect(metodos.parentElement).toBe(cobrar.parentElement);
    expect(within(pie).queryByRole('button', { name: 'Todo lo que falta' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Todo lo que falta' })).toBeInTheDocument();
  });

  // Con el ✕ de cerrar la hoja arriba, un segundo ✕ junto a los modos se confundía con él.
  test('dejar de dividir se dice con palabras, no con otra ✕', async () => {
    const u = userEvent.setup();
    pinta(<CobrarSheet pantalla="pos" order={board()} onClose={() => {}} onCobrado={() => {}} />);
    await openByProducts(u);
    expect(screen.getByRole('button', { name: 'Dejar de dividir' })).toHaveTextContent('Dejar de dividir');
  });

  // Sin botón para cerrar, quien no sabe que se cierra tocando afuera se queda atrapado.
  test('tiene un botón visible para cerrar, de 44 px', async () => {
    const u = userEvent.setup();
    const onClose = vi.fn();
    pinta(<CobrarSheet pantalla="pos" order={board()} onClose={onClose} onCobrado={() => {}} />);
    const cerrar = await screen.findByRole('button', { name: 'Cerrar cobro' });
    expect(parseInt(getComputedStyle(cerrar).minHeight, 10)).toBeGreaterThanOrEqual(44);
    expect(parseInt(getComputedStyle(cerrar).minWidth, 10)).toBeGreaterThanOrEqual(44);
    await u.click(cerrar);
    expect(onClose).toHaveBeenCalled();
  });
});
