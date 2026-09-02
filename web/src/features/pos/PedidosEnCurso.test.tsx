import { vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';

const openOrders = vi.hoisted(() => vi.fn());
vi.mock('../../api/pos', () => ({ posApi: { openOrders } }));

import { PedidosEnCurso } from './PedidosEnCurso';

function pinta(nodo: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <ChakraProvider value={defaultSystem}>
      <QueryClientProvider client={qc}>{nodo}</QueryClientProvider>
    </ChakraProvider>,
  );
}

const pedido = (over: Record<string, unknown> = {}) => ({
  id: 1, number: 12, folioName: 'Tigre', status: 'abierta', serviceType: 'mostrador',
  deliveryPlatformId: null, customerName: null, total: '250', currency: 'MXN',
  paid: false, outstanding: '250', openedAt: new Date().toISOString(),
  enPreparacion: true, renglones: 3, ...over,
});

afterEach(() => { openOrders.mockReset(); });

// El chip es el camino de UN TOQUE a un pedido en curso. Antes el pedido desaparecía al mandarlo y
// volver a él costaba cinco toques por un camino enterrado en la hoja de cobro — que en producción
// nadie usó jamás.
test('pinta un chip por pedido, con su folio y su monto', async () => {
  openOrders.mockResolvedValue({ items: [pedido()], outstanding: '250' });
  pinta(<PedidosEnCurso onAbrir={() => {}} />);

  expect(await screen.findByText('Tigre')).toBeInTheDocument();
  // El monto del chip y el total en riesgo coinciden con un solo pedido: se buscan los dos.
  expect(screen.getAllByText(/250/).length).toBeGreaterThanOrEqual(1);
  // El chip cumple el mínimo táctil: por debajo el dedo falla y la venta cae en otra mesa.
  expect(screen.getByRole('button', { name: /Tigre/ })).toHaveStyle({ minHeight: '44px' });
});

// Un contador en cero es chrome que le quita ancho a la barra en una tableta de 7".
test('no pinta nada cuando no hay pedidos en curso', async () => {
  openOrders.mockResolvedValue({ items: [], outstanding: '0' });
  const { container } = pinta(<PedidosEnCurso onAbrir={() => {}} />);
  await new Promise((r) => setTimeout(r, 0));
  expect(container.querySelector('button')).toBeNull();
});

// EL ENTREGADO SIN COBRAR NO ES UN CHIP: ES UNA PÍLDORA DE DINERO.
//
// Los dos estaban inline y se comían la barra — en la tableta real, tres de éstos dejaban la cuenta
// activa cortada y el último chip truncado a media palabra. Y no son la misma cosa: al chip en
// preparación se le TOCA para agregarle, mientras que el entregado sin cobrar es un aviso de dinero
// que ya se fue con el cliente. Se cuenta en la píldora y se detalla al abrirla.
test('el entregado sin cobrar se cuenta en la píldora, no ocupa un chip', async () => {
  openOrders.mockResolvedValue({
    items: [pedido({ id: 2, folioName: 'Lobo', status: 'entregada', enPreparacion: false })],
    outstanding: '250',
  });
  pinta(<PedidosEnCurso onAbrir={() => {}} />);

  // No hay chip con su nombre: no es algo a lo que se le agregue.
  await screen.findByRole('button', { name: /250/ });
  expect(screen.queryByRole('button', { name: /Lobo/ })).toBeNull();
  // Pero el monto sí está a la vista, que es lo que la píldora existe para gritar.
  expect(screen.getByText(/\(1\)/)).toBeInTheDocument();
});

// CON MUCHOS PEDIDOS, LA FILA NO PUEDE EMPUJAR NADA FUERA DE LA PANTALLA.
//
// Cada chip mide ~150px y no encoge. Sin una caja propia que scrollee, seis pedidos —el máximo de
// un día en producción— empujaban los botones de precios y edición fuera del `overflow: hidden` del
// contenedor: no se veían Y no se podían tocar. Y las cuentas locales, lo único elástico de la
// fila, se aplastaban a cero: el operador dejaba de poder cambiar de cuenta.
test('los chips scrollean dentro de su propia caja acotada', async () => {
  openOrders.mockResolvedValue({
    items: Array.from({ length: 6 }, (_, i) => pedido({ id: i + 1, folioName: `Animal${i}` })),
    outstanding: '1500',
  });
  const { container } = pinta(<PedidosEnCurso onAbrir={() => {}} />);

  await screen.findByText('Animal0');
  const caja = container.querySelector('[class*="css-"]') as HTMLElement | null;
  const conScroll = Array.from(container.querySelectorAll('*')).find(
    (el) => getComputedStyle(el as HTMLElement).overflowX === 'auto',
  );
  expect(conScroll, 'los chips necesitan su propia caja con overflowX auto').toBeTruthy();
  expect(caja).toBeTruthy();
});

// EL NÚMERO DE LA PÍLDORA Y LA LISTA QUE ABRE SALEN DEL MISMO PREDICADO.
//
// La píldora tomaba el total del servidor —que suma TODO lo pendiente— y la lista se armaba con otro
// filtro. En la tableta real decían $2,141 y $1,928: el operador ve dos cifras del mismo dinero y no
// tiene forma de saber cuál miente. Es el corolario del principio III, y ya costó un turno con
// $4,500 de faltante inexplicable.
test('el total de la píldora es la suma de lo que la lista muestra', async () => {
  openOrders.mockResolvedValue({
    items: [
      pedido({ id: 1, folioName: 'Castor', enPreparacion: true, outstanding: '213', total: '213' }),
      pedido({ id: 2, folioName: 'Urraca', enPreparacion: false, status: 'entregada', outstanding: '712', total: '712' }),
    ],
    // El servidor manda una cifra DISTINTA de la suma de la lista, a propósito: es la única forma
    // de saber cuál de las dos fuentes está usando la píldora. Con las dos iguales, el test pasa
    // con el defecto puesto.
    outstanding: '99999',
  });
  pinta(<PedidosEnCurso onAbrir={() => {}} />);

  // Se abre la lista y se compara su encabezado con el de la píldora: son la misma cifra o una de
  // las dos miente.
  // La píldora lleva el conteo entre paréntesis; los chips no.
  const pildora = await screen.findByRole('button', { name: /\(2\)/ });
  expect(pildora.textContent, 'la píldora suma lo que la lista muestra, no lo que manda el servidor').toMatch(/925/);
  expect(pildora.textContent).not.toMatch(/99,?999/);
});

// SOLO SE VEN COMO CHIP LOS QUE SIGUEN EN CURSO Y NO ESTÁN SALDADOS.
//
// Un pedido pagado que sigue en cocina no tiene nada que el cajero deba hacerle: ya cobró. Ponerlo
// como chip satura la barra —la pantalla más apretada del sistema— con algo que no es accionable
// desde ahí. La cocina lo sigue viendo en su tablero, que es donde importa.
test('un pedido en curso YA PAGADO no ocupa un chip', async () => {
  openOrders.mockResolvedValue({
    items: [
      pedido({ id: 1, folioName: 'Pagado', enPreparacion: true, outstanding: '0', total: '100' }),
      pedido({ id: 2, folioName: 'Debe', enPreparacion: true, outstanding: '100', total: '100' }),
    ],
    outstanding: '100',
  });
  pinta(<PedidosEnCurso onAbrir={() => {}} />);

  expect(await screen.findByRole('button', { name: /Debe/ })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /Pagado/ })).toBeNull();
});
