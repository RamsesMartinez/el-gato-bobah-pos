import { vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';
import type { BoardOrder } from '../../types/pos';

const openOrders = vi.hoisted(() => vi.fn());
const order = vi.hoisted(() => vi.fn());
const paymentMethods = vi.hoisted(() => vi.fn());
const chargeOrder = vi.hoisted(() => vi.fn());
vi.mock('../../api/pos', () => ({ posApi: { openOrders, order, paymentMethods, chargeOrder } }));

import { PedidosEnCurso } from './PedidosEnCurso';

function pinta(nodo: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <ChakraProvider value={defaultSystem}>
      <QueryClientProvider client={qc}>{nodo}</QueryClientProvider>
    </ChakraProvider>,
  );
}

const pedido = (over: Partial<BoardOrder> = {}): BoardOrder => ({
  id: 1, number: 12, folioName: 'Tigre', status: 'abierta', serviceType: 'mostrador',
  deliveryPlatformId: null, customerName: null, total: '250', currency: 'MXN' as const,
  paid: false, outstanding: '250', openedAt: new Date().toISOString(),
  enPreparacion: true, renglones: 3, lines: [], ...over,
});

afterEach(() => { openOrders.mockReset(); order.mockReset(); paymentMethods.mockReset(); chargeOrder.mockReset(); });

// LA BARRA ES UN SOLO BOTÓN, y no es una preferencia estética.
//
// Medido en un navegador sin cabeza contra el presupuesto real —1024×600, panel del pedido abierto,
// rol de gerente— la fila pedía 667.6 px sobre 612.6 disponibles: se desbordaba estando VACÍA, y el
// overflow oculto del padre dejaba los botones de precios y edición al 11% visible con un control
// por pedido, y al 0% con dos.
test('la barra ocupa un solo control, no uno por pedido', async () => {
  openOrders.mockResolvedValue({
    items: Array.from({ length: 6 }, (_, i) => pedido({ id: i + 1, folioName: `Animal${i}` })),
    outstanding: '1500',
  });
  const { container } = pinta(<PedidosEnCurso onAbrir={() => {}} onCobrado={() => {}} hayQueAgregar />);

  await screen.findByRole('button', { name: /1,500/ });
  expect(container.querySelectorAll('button')).toHaveLength(1);
});

// Un contador en cero es chrome que le quita ancho a la barra en una tableta de 7", y la caja de
// controles llegaba a cobrar 112 px estando vacía porque vivía fuera del condicional.
test('no pinta nada cuando no hay nada que cobrar', async () => {
  openOrders.mockResolvedValue({ items: [], outstanding: '0' });
  const { container } = pinta(<PedidosEnCurso onAbrir={() => {}} onCobrado={() => {}} hayQueAgregar />);
  await new Promise((r) => setTimeout(r, 0));
  expect(container.querySelector('button')).toBeNull();
});

// EL BOTÓN Y LA LISTA QUE ABRE SALEN DEL MISMO PREDICADO.
//
// El encabezado tomaba el total del servidor —que suma TODO lo pendiente— y la lista se armaba con
// otro filtro. En la tableta real decían $2,141 y $1,928: el operador ve dos cifras del mismo dinero
// y no tiene forma de saber cuál miente. Es el corolario del principio III, y ya costó un turno con
// $4,500 de faltante inexplicable.
test('el total del botón es la suma de lo que la lista muestra', async () => {
  openOrders.mockResolvedValue({
    items: [
      pedido({ id: 1, folioName: 'Castor', enPreparacion: true, outstanding: '213', total: '213' }),
      pedido({ id: 2, folioName: 'Urraca', enPreparacion: false, status: 'entregada', outstanding: '712', total: '712' }),
    ],
    // El servidor manda una cifra DISTINTA de la suma de la lista, a propósito: es la única forma
    // de saber cuál de las dos fuentes está usando el botón. Con las dos iguales, el test pasa con
    // el defecto puesto.
    outstanding: '99999',
  });
  pinta(<PedidosEnCurso onAbrir={() => {}} onCobrado={() => {}} hayQueAgregar />);

  const boton = await screen.findByRole('button', { name: /\(2\)/ });
  expect(boton.textContent, 'el botón suma lo que la lista muestra, no lo que manda el servidor').toMatch(/925/);
  expect(boton.textContent).not.toMatch(/99,?999/);
});

// AGREGARLE A UN PEDIDO QUE SIGUE EN COCINA ES LO QUE ESTA HOJA NO PUEDE PERDER.
//
// La hoja ya solo trae lo que falta por cobrar, y de ahí a "esta hoja es solo para cobrar" hay un
// paso. Pero el pedido que sigue en la plancha y todavía debe es justo al que el cliente le pide una
// más, y este es el único camino de la app para llevárselo: el tablero de Pedidos entrega, cobra y
// cancela, pero no agrega.
test('al pedido que sigue en cocina se le puede agregar', async () => {
  const u = userEvent.setup();
  const onAbrir = vi.fn();
  openOrders.mockResolvedValue({
    items: [pedido({ id: 7, folioName: 'Castor', enPreparacion: true, outstanding: '250' })],
    outstanding: '250',
  });
  pinta(<PedidosEnCurso onAbrir={onAbrir} onCobrado={() => {}} hayQueAgregar />);

  await u.click(await screen.findByRole('button', { name: /250/ }));
  await u.click(await screen.findByRole('button', { name: /Agregar/ }));
  expect(onAbrir).toHaveBeenCalledWith(expect.objectContaining({ id: 7 }));
});

// El entregado sin cobrar es el caso caro —el cliente ya se fue con la comida— y necesita decirse
// con todas sus letras, no depender del color.
//
// Y SÍ ofrece agregarle: el cliente que ya recibió lo suyo y sigue en la mesa pide una más. Mandar
// eso como pedido aparte deja dos cuentas para la misma mesa y una de las dos se pierde de vista.
test('al entregado sin cobrar se le puede agregar y cobrar, y se dice que ya salió', async () => {
  const u = userEvent.setup();
  const onAbrir = vi.fn();
  openOrders.mockResolvedValue({
    items: [pedido({ id: 2, folioName: 'Lobo', status: 'entregada', enPreparacion: false })],
    outstanding: '250',
  });
  pinta(<PedidosEnCurso onAbrir={onAbrir} onCobrado={() => {}} hayQueAgregar />);

  await u.click(await screen.findByRole('button', { name: /250/ }));
  expect(await screen.findByText('Lobo')).toBeInTheDocument();
  expect(screen.getByText(/ya se entregó/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /Cobrar \$250/ })).toBeInTheDocument();

  await u.click(screen.getByRole('button', { name: /Agregar/ }));
  expect(onAbrir).toHaveBeenCalledWith(expect.objectContaining({ id: 2 }));
});

// Un control de 44 px que no hace nada ni dice por qué es peor que no tenerlo: con la cuenta vacía,
// tocarlo hacía un `return` mudo.
test('sin productos capturados, Agregar está apagado y la hoja dice por qué', async () => {
  const u = userEvent.setup();
  openOrders.mockResolvedValue({ items: [pedido()], outstanding: '250' });
  pinta(<PedidosEnCurso onAbrir={() => {}} onCobrado={() => {}} hayQueAgregar={false} />);

  await u.click(await screen.findByRole('button', { name: /250/ }));
  expect(await screen.findByText(/Captura los productos en una cuenta/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /Agregar/ })).toBeDisabled();
});

// LA HOJA NO VUELVE A PLEGAR NADA, PORQUE YA NO LE LLEGA NADA QUE PLEGAR.
//
// Medido en el ambiente de pruebas: 30 renglones, 14 de ellos ya cobrados, en una pantalla donde
// caben cinco. Primero se plegaron aquí; ahora el servidor no los manda, que es lo único que deja al
// encabezado y a la lista contando lo mismo.
//
// El precio se pagó y hay que saberlo: al pedido YA SALDADO que sigue en cocina se le podía agregar
// desde esta hoja, y era su único camino. Ahora esa venta se captura aparte.
test('la hoja pinta un renglón por pedido, sin plegados', async () => {
  const u = userEvent.setup();
  openOrders.mockResolvedValue({
    items: [
      pedido({ id: 1, folioName: 'Debe', outstanding: '250', total: '250' }),
      pedido({ id: 2, folioName: 'TambienDebe', outstanding: '100', total: '100' }),
    ],
    outstanding: '350',
  });
  pinta(<PedidosEnCurso onAbrir={() => {}} onCobrado={() => {}} hayQueAgregar />);

  await u.click(await screen.findByRole('button', { name: /350/ }));
  expect(await screen.findByText('Debe')).toBeInTheDocument();
  expect(screen.getByText('TambienDebe')).toBeInTheDocument();
  expect(screen.queryByText(/ya cobrado/)).toBeNull();
});

// COBRAR DESDE ESTA HOJA TERMINA LA VENTA DONDE SE IMPRIME EL TICKET.
//
// La hoja se quedaba refrescando su lista y nada más: quien cobraba desde el botón naranja no veía
// la confirmación ni podía sacar el papel, mientras que cobrar el mismo pedido desde el panel sí lo
// ofrecía. El ticket no lo pinta este componente —vive en la pantalla que lo monta— justamente para
// que los dos caminos terminen en la MISMA confirmación y ninguno se quede sin papel.
test('al quedar saldado, avisa a la pantalla para que ofrezca el ticket', async () => {
  const u = userEvent.setup();
  const onCobrado = vi.fn();
  openOrders.mockResolvedValue({
    items: [pedido({ id: 7, folioName: 'Tigre', outstanding: '250', total: '250' })],
    outstanding: '250',
  });
  order.mockResolvedValue({ ...pedido({ id: 7, outstanding: '250', total: '250' }), lines: [] });
  paymentMethods.mockResolvedValue({
    items: [{ id: 1, name: 'Efectivo', kind: 'efectivo', deliveryPlatformId: null }],
  });
  chargeOrder.mockResolvedValue({ outstanding: '0', paid: true, yaEstaba: false });

  pinta(<PedidosEnCurso onAbrir={() => {}} onCobrado={onCobrado} hayQueAgregar />);

  await u.click(await screen.findByRole('button', { name: /250/ }));
  await u.click(await screen.findByRole('button', { name: /^Cobrar \$250/ }));
  await u.click(await screen.findByRole('button', { name: 'Efectivo' }));
  await u.click(screen.getByRole('button', { name: /^Cobrar \$250/ }));

  await waitFor(() => expect(onCobrado).toHaveBeenCalledWith(
    expect.objectContaining({ paid: true }), 7,
  ));
});

// ABRIR LA HOJA DE COBRO CIERRA LA LISTA EN EL MISMO TOQUE, Y LAS DOS COSAS TIENEN QUE OCURRIR.
//
// Es una actualización de React que cierra una hoja y monta otra a la vez. Si la que se monta nace
// con `open` ya puesto —sin transición de cerrado a abierto— la librería no llega a montarla: con
// Chakra 3.37 quedaban CERO diálogos en el árbol, o sea que tocar Cobrar no abría nada y el
// operador se quedaba con el cliente enfrente y sin pantalla.
//
// Con 3.36 el mismo código funcionaba. Este test es lo que impide que un bump de dependencias
// vuelva a tumbar el camino con el que se cobra desde el botón naranja.
test('tocar Cobrar cierra la lista Y abre la hoja de cobro', async () => {
  const u = userEvent.setup();
  openOrders.mockResolvedValue({
    items: [pedido({ id: 7, folioName: 'Tigre', outstanding: '250', total: '250' })],
    outstanding: '250',
  });
  order.mockResolvedValue({ ...pedido({ id: 7, outstanding: '250', total: '250' }), lines: [] });
  paymentMethods.mockResolvedValue({
    items: [{ id: 1, name: 'Efectivo', kind: 'efectivo', deliveryPlatformId: null }],
  });
  pinta(<PedidosEnCurso onAbrir={() => {}} onCobrado={() => {}} hayQueAgregar />);

  await u.click(await screen.findByRole('button', { name: /250/ }));
  await u.click(await screen.findByRole('button', { name: /^Cobrar \$250/ }));

  // La hoja de cobro se reconoce por su encabezado de dos cifras, que la lista no tiene.
  expect(await screen.findByText(/Falta \$250/)).toBeInTheDocument();
  expect(screen.getByText(/Total \$250/)).toBeInTheDocument();
});
