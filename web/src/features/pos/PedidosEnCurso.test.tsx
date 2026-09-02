import { vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';
import type { BoardOrder } from '../../types/pos';

const openOrders = vi.hoisted(() => vi.fn());
vi.mock('../../api/pos', () => ({ posApi: { openOrders, order: vi.fn(), paymentMethods: vi.fn(), chargeOrder: vi.fn() } }));

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

afterEach(() => { openOrders.mockReset(); });

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
  const { container } = pinta(<PedidosEnCurso onAbrir={() => {}} hayQueAgregar />);

  await screen.findByRole('button', { name: /1,500/ });
  expect(container.querySelectorAll('button')).toHaveLength(1);
});

// Un contador en cero es chrome que le quita ancho a la barra en una tableta de 7", y la caja de
// controles llegaba a cobrar 112 px estando vacía porque vivía fuera del condicional.
test('no pinta nada cuando no hay nada que cobrar', async () => {
  openOrders.mockResolvedValue({ items: [], outstanding: '0' });
  const { container } = pinta(<PedidosEnCurso onAbrir={() => {}} hayQueAgregar />);
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
  pinta(<PedidosEnCurso onAbrir={() => {}} hayQueAgregar />);

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
  pinta(<PedidosEnCurso onAbrir={onAbrir} hayQueAgregar />);

  await u.click(await screen.findByRole('button', { name: /250/ }));
  await u.click(await screen.findByRole('button', { name: /Agregar/ }));
  expect(onAbrir).toHaveBeenCalledWith(expect.objectContaining({ id: 7 }));
});

// El entregado sin cobrar es el caso caro —el cliente ya se fue con la comida— y necesita decirse
// con todas sus letras, no depender del color.
test('el entregado sin cobrar se nombra y solo ofrece cobrar', async () => {
  const u = userEvent.setup();
  openOrders.mockResolvedValue({
    items: [pedido({ id: 2, folioName: 'Lobo', status: 'entregada', enPreparacion: false })],
    outstanding: '250',
  });
  pinta(<PedidosEnCurso onAbrir={() => {}} hayQueAgregar />);

  await u.click(await screen.findByRole('button', { name: /250/ }));
  expect(await screen.findByText('Lobo')).toBeInTheDocument();
  expect(screen.getByText(/ya se entregó/)).toBeInTheDocument();
  // No se le puede agregar: el servidor rechaza renglones en un pedido entregado.
  expect(screen.queryByRole('button', { name: /Agregar/ })).toBeNull();
  expect(screen.getByRole('button', { name: /Cobrar \$250/ })).toBeInTheDocument();
});

// Un control de 44 px que no hace nada ni dice por qué es peor que no tenerlo: con la cuenta vacía,
// tocarlo hacía un `return` mudo.
test('sin productos capturados, Agregar está apagado y la hoja dice por qué', async () => {
  const u = userEvent.setup();
  openOrders.mockResolvedValue({ items: [pedido()], outstanding: '250' });
  pinta(<PedidosEnCurso onAbrir={() => {}} hayQueAgregar={false} />);

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
  pinta(<PedidosEnCurso onAbrir={() => {}} hayQueAgregar />);

  await u.click(await screen.findByRole('button', { name: /350/ }));
  expect(await screen.findByText('Debe')).toBeInTheDocument();
  expect(screen.getByText('TambienDebe')).toBeInTheDocument();
  expect(screen.queryByText(/ya cobrado/)).toBeNull();
});
