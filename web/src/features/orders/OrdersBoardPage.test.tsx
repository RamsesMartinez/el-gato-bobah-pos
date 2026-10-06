import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';

import { Provider } from '../../components/ui/provider';
import { useSessionStore } from '../../stores/session';
import type { BoardLine, BoardOrder } from '../../types/pos';

const api = vi.hoisted(() => ({
  businessSettings: vi.fn(),
  activeOrders: vi.fn(),
  deliveredOrders: vi.fn(),
  deliverOrder: vi.fn(),
  deliverLine: vi.fn(),
  cancelOrder: vi.fn(),
  cancelOrderLine: vi.fn(),
  cancelPendingLines: vi.fn(),
  refundOrder: vi.fn(),
  order: vi.fn(),
  paymentMethods: vi.fn(),
  chargeOrder: vi.fn(),
  setOrderDiscount: vi.fn(),
}));
vi.mock('../../api/pos', () => ({ posApi: api }));
vi.mock('../../hooks/useOrderEvents', () => ({ useOrderEvents: () => true }));
const toast = vi.hoisted(() => vi.fn());
vi.mock('../../components/ui/toaster', () => ({ toaster: { create: toast } }));

import { OrdersBoardPage } from './OrdersBoardPage';

const linea = (id: number, qty: number, delivered = 0): BoardLine => ({
  id, name: `Producto ${id}`, qty: String(qty), delivered: String(delivered),
});

const pedido = (over: Partial<BoardOrder> = {}): BoardOrder => ({
  id: 5, number: 3, folioName: 'Persa', status: 'abierta', serviceType: 'mostrador',
  deliveryPlatformId: null, customerName: null, total: '110', currency: 'MXN' as const,
  paid: false, outstanding: '110', openedAt: new Date().toISOString(),
  enPreparacion: true, renglones: 1, lines: [linea(1, 1)], ...over,
});

function entrar(permissions: string[]) {
  useSessionStore.setState({
    token: 't', status: 'authed',
    user: { id: 1, companyId: 2, name: 'Carlos', role: 'cajero', permissions },
  });
}

function pintar(o: BoardOrder, { cobra = false } = {}) {
  api.businessSettings.mockResolvedValue({ kitchenCanCharge: cobra, timezone: 'America/Mexico_City', corteDeVista: '' });
  api.activeOrders.mockResolvedValue({ items: [o] });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const nodo: ReactNode = <QueryClientProvider client={qc}><OrdersBoardPage /></QueryClientProvider>;
  return render(<Provider>{nodo}</Provider>);
}

const tarjeta = async (folio = 'Persa') => {
  const titulo = await screen.findByText(folio);
  return titulo.closest('[data-order-card]') as HTMLElement;
};

beforeEach(() => {
  entrar(['orders.cancel_pending', 'orders.move_lines']);
  api.deliveredOrders.mockResolvedValue({ items: [] });
  api.paymentMethods.mockResolvedValue({ items: [] });
});
afterEach(() => {
  vi.clearAllMocks();
  useSessionStore.setState({ user: null, token: null });
});

// NINGUNA TARJETA SIN SALIDA (SC-003 en pantalla).
//
// El incidente: un pedido con todo lo vivo entregado y sin deuda se quedó en el tablero sin un solo
// botón que lo cerrara, y la única salida era cancelarlo —que el servidor rechaza porque ya salió
// comida—. Cada combinación de «falta entregar» × «debe» × «el tablero cobra», más las dos de un
// pedido sin productos, tiene que ofrecer algo que lo cierre o decir dónde se cierra.
describe('la tarjeta ofrece una salida en cada combinación', () => {
  const todoEntregado = { lines: [linea(1, 2, 2)] };
  const conPendiente = { lines: [linea(1, 2, 1)] };
  const sinDeuda = { outstanding: '0', paid: true };

  type Caso = { nombre: string; o: Partial<BoardOrder>; cobra: boolean; espera: (c: HTMLElement) => void };
  const casos: Caso[] = [
    ...[false, true].flatMap((debe) => [false, true].map((cobra): Caso => ({
      nombre: `falta entregar · ${debe ? 'debe' : 'no debe'} · ${cobra ? 'cobra' : 'no cobra'}`,
      o: { ...conPendiente, ...(debe ? {} : sinDeuda) },
      cobra,
      espera: (c) => {
        expect(within(c).getByRole('button', { name: 'Entregar todo' })).toBeInTheDocument();
        expect(within(c).queryByRole('button', { name: /^Cobrar/ }) !== null).toBe(debe && cobra);
        expect(within(c).queryByRole('button', { name: 'Cerrar pedido' })).toBeNull();
      },
    }))),
    ...[false, true].map((cobra): Caso => ({
      nombre: `todo entregado · no debe · ${cobra ? 'cobra' : 'no cobra'}`,
      o: { ...todoEntregado, ...sinDeuda },
      cobra,
      espera: (c) => {
        expect(within(c).getByRole('button', { name: 'Cerrar pedido' })).toBeInTheDocument();
        expect(within(c).queryByRole('button', { name: 'Entregar todo' })).toBeNull();
      },
    })),
    {
      nombre: 'todo entregado · debe · cobra',
      o: { ...todoEntregado, outstanding: '65' },
      cobra: true,
      espera: (c) => {
        expect(within(c).getByRole('button', { name: /^Cobrar \$65/ })).toBeInTheDocument();
        expect(within(c).queryByRole('button', { name: 'Cerrar pedido' })).toBeNull();
      },
    },
    {
      nombre: 'todo entregado · debe · no cobra',
      o: { ...todoEntregado, outstanding: '65' },
      cobra: false,
      espera: (c) => {
        expect(within(c).getByText(/^Falta cobrar \$65.* en caja$/)).toBeInTheDocument();
        expect(within(c).queryByRole('button', { name: /^Cobrar/ })).toBeNull();
        expect(within(c).queryByRole('button', { name: 'Cerrar pedido' })).toBeNull();
      },
    },
    {
      nombre: 'sin productos · sin pagos',
      o: { lines: [], total: '0', outstanding: '0', paid: true },
      cobra: false,
      espera: (c) => {
        expect(within(c).getByRole('button', { name: 'Cerrar pedido' })).toBeInTheDocument();
        expect(within(c).queryByRole('button', { name: 'Entregar todo' })).toBeNull();
      },
    },
    {
      // Lo único que queda es el envío, y ya se cobró: hay dinero que devolver antes de cerrarlo.
      nombre: 'sin productos · con pagos',
      o: { lines: [], total: '30', outstanding: '0', paid: true },
      cobra: true,
      espera: (c) => {
        expect(within(c).getByText(/^Tiene pagos por devolver/)).toBeInTheDocument();
        expect(within(c).queryByRole('button', { name: 'Cerrar pedido' })).toBeNull();
        expect(within(c).queryByRole('button', { name: 'Entregar todo' })).toBeNull();
      },
    },
  ];

  test('son diez combinaciones', () => expect(casos).toHaveLength(10));

  test.each(casos)('$nombre', async ({ o, cobra, espera }) => {
    pintar(pedido(o), { cobra });
    // El ajuste de cobrar llega en su propia petición; sin esperarla, el caso «cobra» se evaluaría
    // con el tablero creyendo que no cobra.
    await waitFor(() => expect(api.businessSettings).toHaveBeenCalled());
    const c = await tarjeta();
    await waitFor(() => espera(c));
  });
});

describe('qué llama «Cerrar pedido»', () => {
  test('con todo entregado y sin deuda, entrega el pedido', async () => {
    const u = userEvent.setup();
    api.deliverOrder.mockResolvedValue(undefined);
    pintar(pedido({ lines: [linea(1, 1, 1)], outstanding: '0', paid: true }));

    await u.click(within(await tarjeta()).getByRole('button', { name: 'Cerrar pedido' }));

    await waitFor(() => expect(api.deliverOrder).toHaveBeenCalledWith(5));
    expect(api.cancelPendingLines).not.toHaveBeenCalled();
  });

  // Sin productos, el motivo es fijo («Sin productos») y lo pone el servidor; pedirlo aquí sería un
  // toque que no decide nada.
  test('sin productos y sin pagos, lo cierra sin pedir motivo', async () => {
    const u = userEvent.setup();
    api.cancelPendingLines.mockResolvedValue({ removed: 0, restocked: 0 });
    pintar(pedido({ lines: [], total: '0', outstanding: '0', paid: true }));

    await u.click(within(await tarjeta()).getByRole('button', { name: 'Cerrar pedido' }));

    await waitFor(() => expect(api.cancelPendingLines).toHaveBeenCalled());
    expect(api.cancelPendingLines.mock.calls[0]).toEqual([5]);
    expect(api.deliverOrder).not.toHaveBeenCalled();
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });
});

describe('el menú de la tarjeta', () => {
  const abrirMenu = async (u: ReturnType<typeof userEvent.setup>) => {
    await u.click(within(await tarjeta()).getByRole('button', { name: 'Más' }));
  };

  // Quien no puede cancelar el pedido sí tiene que poder quitar lo que no se va a entregar: son dos
  // permisos, y antes la única salida era «Cancelar pedido», que rebotaba con 403.
  test('«Quitar lo que falta» va aparte de «Cancelar pedido», cada uno con su permiso', async () => {
    const u = userEvent.setup();
    pintar(pedido({ lines: [linea(1, 2)] }));
    await abrirMenu(u);

    expect(await screen.findByRole('menuitem', { name: 'Quitar lo que falta' })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: 'Cancelar pedido' })).toBeNull();
  });

  test('«Cancelar pedido» se ofrece solo con su permiso', async () => {
    const u = userEvent.setup();
    entrar(['orders.cancel']);
    pintar(pedido({ lines: [linea(1, 2)] }));
    await abrirMenu(u);

    expect(await screen.findByRole('menuitem', { name: 'Cancelar pedido' })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: 'Quitar lo que falta' })).toBeNull();
  });

  test('«Quitar lo que falta» abre su hoja y manda el motivo elegido', async () => {
    const u = userEvent.setup();
    api.cancelPendingLines.mockResolvedValue({ removed: 2, restocked: 1 });
    pintar(pedido({ lines: [linea(1, 1, 1), linea(2, 1), linea(3, 1)] }));
    await abrirMenu(u);
    await u.click(await screen.findByRole('menuitem', { name: 'Quitar lo que falta' }));

    // La hoja se monta en el mismo toque que cierra el menú: sin montar cerrada, no aparece.
    const hoja = await screen.findByRole('dialog');
    await u.click(within(hoja).getByRole('radio', { name: 'Sin insumos' }));
    await u.click(within(hoja).getByRole('button', { name: 'Quitar los 2 que faltan' }));

    await waitFor(() => expect(api.cancelPendingLines).toHaveBeenCalledWith(5, 'Sin insumos'));
    await waitFor(() => expect(document.querySelector('[role="dialog"]')).toBeNull());
  });

  // Cancelar un pedido del que ya salió comida lo rechaza el servidor. Ofrecer la hoja de cancelar
  // ahí era mandar al operador a un 409; lo que sí puede hacer es quitar lo que falta.
  test('«Cancelar pedido» con algo entregado abre la hoja de quitar lo que falta', async () => {
    const u = userEvent.setup();
    entrar(['orders.cancel', 'orders.cancel_pending']);
    pintar(pedido({ lines: [linea(1, 1, 1), ...Array.from({ length: 10 }, (_, i) => linea(i + 2, 1))] }));
    await abrirMenu(u);
    await u.click(await screen.findByRole('menuitem', { name: 'Cancelar pedido' }));

    await waitFor(() => expect(document.querySelector('[role="dialog"]')).not.toBeNull());
    expect(within(screen.getByRole('dialog')).getByRole('button', { name: 'Quitar los 10 que faltan' }))
      .toBeInTheDocument();
  });
});

// La mesa del incidente medía ~620 px con once productos: en una tableta de 600 px los botones
// quedaban debajo del borde y la tarjeta no tenía salida a la vista.
test('una tarjeta de 11 productos deja los botones a la vista: la lista hace scroll propio', async () => {
  pintar(pedido({ lines: Array.from({ length: 11 }, (_, i) => linea(i + 1, 1)) }));
  const c = await tarjeta();

  const lista = within(c).getByRole('list', { name: 'Falta por entregar' });
  expect(lista).toHaveStyle({ overflowY: 'auto' });
  expect(getComputedStyle(lista).maxHeight).toMatch(/dvh$/);
  expect(within(lista).queryByRole('button', { name: 'Entregar todo' })).toBeNull();
  expect(within(c).getByRole('button', { name: 'Entregar todo' })).toBeInTheDocument();
});

test('con pocos productos la lista no se recorta', async () => {
  pintar(pedido({ lines: Array.from({ length: 5 }, (_, i) => linea(i + 1, 1)) }));
  const lista = within(await tarjeta()).getByRole('list', { name: 'Falta por entregar' });
  expect(getComputedStyle(lista).maxHeight).not.toMatch(/dvh$/);
});

// «Renglón» es palabra de quien programa; quien opera quita productos.
test('quitar un producto avisa «Producto quitado»', async () => {
  const u = userEvent.setup();
  api.cancelOrderLine.mockResolvedValue({ repusoInventario: true });
  pintar(pedido({ lines: [linea(1, 1), linea(2, 1)] }));

  await u.click(within(await tarjeta()).getByRole('button', { name: 'Quitar Producto 1' }));
  const hoja = await screen.findByRole('dialog');
  await u.click(within(hoja).getByRole('radio', { name: 'Ya no lo quiere' }));
  await u.click(within(hoja).getByRole('button', { name: 'Quitar del pedido' }));

  await waitFor(() => expect(toast).toHaveBeenCalledWith(expect.objectContaining({ title: 'Producto quitado' })));
});

// Las dos acciones del menú que quitan algo no pesan lo mismo: «Quitar lo que falta» deja vivo el
// pedido y lo entregado; «Cancelar pedido» lo tira entero. Pegadas y del mismo rojo, un dedo que
// erra por unos píxeles cancela lo que solo quería recortar.
test('en el menú, «Cancelar pedido» va separado de «Quitar lo que falta» y es el único en rojo', async () => {
  const u = userEvent.setup();
  entrar(['orders.cancel', 'orders.cancel_pending']);
  pintar(pedido({ lines: [linea(1, 2)] }));
  await u.click(within(await tarjeta()).getByRole('button', { name: 'Más' }));

  const quitar = await screen.findByRole('menuitem', { name: 'Quitar lo que falta' });
  const cancelar = screen.getByRole('menuitem', { name: 'Cancelar pedido' });
  const entreLosDos = Array.from(document.querySelectorAll('[role="separator"]')).filter((s) =>
    (quitar.compareDocumentPosition(s) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0 &&
    (s.compareDocumentPosition(cancelar) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0);
  expect(entreLosDos, 'no hay separador entre «Quitar lo que falta» y «Cancelar pedido»').toHaveLength(1);
  expect(getComputedStyle(cancelar).color).toMatch(/red/);
  expect(getComputedStyle(quitar).color, '«Quitar lo que falta» no es destructivo y salió en rojo').not.toMatch(/red/);
});

// Un segundo toque mientras la primera petición viaja mandaba otra: con la red lenta, el operador
// cree que no tocó bien y vuelve a tocar.
describe('«Cerrar pedido» no se manda dos veces', () => {
  const casos = [
    { nombre: 'todo entregado (entrega el pedido)', o: { lines: [linea(1, 1, 1)], outstanding: '0', paid: true }, llamada: api.deliverOrder },
    { nombre: 'sin productos (lo cierra vacío)', o: { lines: [], total: '0', outstanding: '0', paid: true }, llamada: api.cancelPendingLines },
  ];
  test.each(casos)('$nombre', async ({ o, llamada }) => {
    const u = userEvent.setup({ pointerEventsCheck: 0 });
    llamada.mockReturnValue(new Promise(() => {}));
    pintar(pedido(o));
    const boton = within(await tarjeta()).getByRole('button', { name: 'Cerrar pedido' });

    await u.click(boton);
    await waitFor(() => expect(boton).toBeDisabled());
    await u.click(boton);

    expect(llamada).toHaveBeenCalledTimes(1);
  });
});

// «Tiene pagos por devolver» a quien no puede devolverlos lo deja frente a una tarjeta sin salida
// y sin saber a quién pedírsela.
describe('a quién toca devolver los pagos de un pedido vacío', () => {
  const vacioCobrado = { lines: [], total: '30', outstanding: '0', paid: true };
  test('sin permiso de devolver, dice que avise al gerente', async () => {
    pintar(pedido(vacioCobrado));
    expect(within(await tarjeta()).getByText('Tiene pagos por devolver: avisa al gerente')).toBeInTheDocument();
  });
  test('con permiso de devolver, basta con decir que tiene pagos', async () => {
    entrar(['payments.void', 'orders.cancel_pending']);
    pintar(pedido(vacioCobrado));
    expect(within(await tarjeta()).getByText('Tiene pagos por devolver')).toBeInTheDocument();
  });
});

// Los botones del renglón medían 32 px de ancho (`2rem`): en 7" el dedo atinaba al vecino, y el
// vecino del bote de quitar es el botón verde de entregar.
test('los botones del renglón miden al menos 44 px de ancho', async () => {
  pintar(pedido({ lines: [linea(1, 3)] }));
  const c = await tarjeta();
  for (const nombre of ['Uno menos', 'Uno más', 'Quitar Producto 1']) {
    expect(getComputedStyle(within(c).getByRole('button', { name: nombre })).minWidth, nombre).toBe('44px');
  }
});
