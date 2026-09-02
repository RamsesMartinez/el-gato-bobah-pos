import { vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';
import type { BoardOrder } from '../../types/pos';

const order = vi.hoisted(() => vi.fn());
const paymentMethods = vi.hoisted(() => vi.fn());
const chargeOrder = vi.hoisted(() => vi.fn());
vi.mock('../../api/pos', () => ({ posApi: { order, paymentMethods, chargeOrder } }));

import { CobrarSheet } from './CobrarSheet';

function pinta(nodo: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <ChakraProvider value={defaultSystem}>
      <QueryClientProvider client={qc}>{nodo}</QueryClientProvider>
    </ChakraProvider>,
  );
}

const pedido = (over: Partial<BoardOrder> = {}): BoardOrder => ({
  id: 7, number: 13, folioName: 'Tigre', status: 'lista', serviceType: 'mostrador',
  deliveryPlatformId: null, customerName: null, total: '500', currency: 'MXN' as const,
  paid: false, outstanding: '500', openedAt: new Date().toISOString(),
  enPreparacion: true, renglones: 3, lines: [], ...over,
});

const metodos = [
  { id: 1, name: 'Efectivo', kind: 'efectivo', deliveryPlatformId: null },
  { id: 2, name: 'Tarjeta', kind: 'tarjeta', deliveryPlatformId: null },
];

beforeEach(() => {
  order.mockResolvedValue({ ...pedido(), lines: [] });
  paymentMethods.mockResolvedValue({ items: metodos });
  chargeOrder.mockReset();
});

// La hoja tiene que decir las DOS cifras. Pintando solo el faltante donde el operador espera el
// total, un pedido de $500 con $300 ya abonados se ve idéntico a uno de $200 y nadie puede notar la
// diferencia desde esta pantalla.
test('el encabezado dice el total del pedido y lo que falta, no solo una', async () => {
  order.mockResolvedValue({ ...pedido({ outstanding: '200' }), lines: [] });
  pinta(<CobrarSheet order={pedido({ outstanding: '200' })} onClose={() => {}} onCobrado={() => {}} />);

  expect(await screen.findByText(/Total \$500/)).toBeInTheDocument();
  expect(screen.getByText(/Falta \$200/)).toBeInTheDocument();
});

// LA CIFRA LA MANDA EL SERVIDOR, no la foto que traía la lista al abrir la hoja.
//
// La hoja recibía el objeto y nunca se actualizaba: un pedido que otra caja cobró entretanto seguía
// diciendo "Falta $500" indefinidamente, y el operador dividía contra un faltante que ya no existía.
test('el faltante sale del pedido vivo, no del que traía la lista', async () => {
  order.mockResolvedValue({ ...pedido({ outstanding: '120' }), lines: [] });
  // La lista traía $500; el servidor dice $120 porque otra caja ya cobró un pedazo.
  pinta(<CobrarSheet order={pedido({ outstanding: '500' })} onClose={() => {}} onCobrado={() => {}} />);

  expect(await screen.findByText(/Falta \$120/)).toBeInTheDocument();
});

// NINGÚN MÉTODO VIENE PRESELECCIONADO, y no es una omisión.
//
// Aquí el pedido ya existe: un dedo que va directo a Cobrar con un método puesto por default
// registra con tarjeta dinero que entró en efectivo, y el corte cierra descuadrado en los dos
// métodos a la vez. El tap sobre el método es la confirmación de con qué se está pagando.
test('no se puede cobrar sin elegir método', async () => {
  pinta(<CobrarSheet order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  const boton = await screen.findByRole('button', { name: /^Cobrar / });
  expect(boton).toBeDisabled();
  expect(screen.getByText('Falta con qué paga.')).toBeInTheDocument();
});

// Dividir no puede costar teclear: el teclado del sistema come 250 de los 600 px de alto de la
// tableta y tapa justo la cifra que decide si el botón se enciende.
test('ofrece cobrar todo o repartir entre dos, tres y cuatro, sin teclado', async () => {
  const u = userEvent.setup();
  pinta(<CobrarSheet order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await screen.findByText(/¿Cuánto cobras ahora\?/);
  expect(screen.getByRole('button', { name: /Entre 2.*250/ })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /Entre 3.*166\.66/ })).toBeInTheDocument();

  await u.click(screen.getByRole('button', { name: /Entre 3.*166\.66/ }));
  await u.click(screen.getByRole('button', { name: 'Tarjeta' }));
  expect(screen.getByRole('button', { name: /^Cobrar \$166\.66/ })).toBeEnabled();
});

// UN COBRO A LA VEZ, con su llave. Mandar N pagos de un golpe registra dinero que todavía no se
// recibió, y no existe forma de deshacer un pago: no hay endpoint que lo quite y el reembolso es de
// la cuenta entera.
test('cobrar un pedazo manda UNA llamada, con su llave de idempotencia', async () => {
  const u = userEvent.setup();
  chargeOrder.mockResolvedValue({ outstanding: '250', paid: false, yaEstaba: false });
  pinta(<CobrarSheet order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: /Entre 2.*250/ }));
  await u.click(screen.getByRole('button', { name: 'Tarjeta' }));
  await u.click(screen.getByRole('button', { name: /^Cobrar \$250/ }));

  await waitFor(() => expect(chargeOrder).toHaveBeenCalledTimes(1));
  const [id, body] = chargeOrder.mock.calls[0];
  expect(id).toBe(7);
  expect(body.amount).toBe(250);
  expect(body.methodId).toBe(2);
  expect(typeof body.clientUuid).toBe('string');
});

// La hoja NO se cierra mientras quede saldo: el siguiente comensal todavía tiene que pagar, y
// cerrarla obligaría a volver a buscar el pedido en la lista con la mesa esperando.
test('con saldo pendiente la hoja sigue abierta y se prepara para el siguiente', async () => {
  const u = userEvent.setup();
  const onClose = vi.fn();
  chargeOrder.mockResolvedValue({ outstanding: '250', paid: false, yaEstaba: false });
  pinta(<CobrarSheet order={pedido()} onClose={onClose} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: /Entre 2.*250/ }));
  await u.click(screen.getByRole('button', { name: 'Tarjeta' }));
  await u.click(screen.getByRole('button', { name: /^Cobrar \$250/ }));

  await waitFor(() => expect(chargeOrder).toHaveBeenCalled());
  expect(onClose).not.toHaveBeenCalled();
  // Y queda constancia de lo que ya entró, para que el operador no tenga que acordarse.
  expect(await screen.findByText(/\$250 Tarjeta/)).toBeInTheDocument();
});

// Saldado el pedido sí se cierra: dejarla abierta sobre algo que ya no debe nada invita a cobrarlo
// otra vez.
test('al quedar saldado se cierra', async () => {
  const u = userEvent.setup();
  const onClose = vi.fn();
  chargeOrder.mockResolvedValue({ outstanding: '0', paid: true, yaEstaba: false });
  pinta(<CobrarSheet order={pedido()} onClose={onClose} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: 'Tarjeta' }));
  await u.click(screen.getByRole('button', { name: /^Cobrar \$500/ }));

  await waitFor(() => expect(onClose).toHaveBeenCalled());
});

// EL CASO CARO: el operador tiene el efectivo del cliente en la mano y el servidor dice que no.
// `String(e)` ahí es un objeto de error crudo en la pantalla de quien tiene que decidir qué hacer.
test('traduce el rebote de otra caja a algo accionable', async () => {
  const u = userEvent.setup();
  chargeOrder.mockRejectedValue(new Error('conflicto: ese pedido ya está cobrado'));
  pinta(<CobrarSheet order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: 'Tarjeta' }));
  await u.click(screen.getByRole('button', { name: /^Cobrar \$500/ }));

  expect(await screen.findByText('Otra caja acaba de cobrar este pedido')).toBeInTheDocument();
});

// En efectivo, el aviso de faltante es el único control que impide cobrar de menos. En modo
// dividido no existía en ninguna de las dos hojas.
test('en efectivo no deja cobrar si lo recibido no alcanza', async () => {
  const u = userEvent.setup();
  pinta(<CobrarSheet order={pedido({ outstanding: '175' })} onClose={() => {}} onCobrado={() => {}} />);
  order.mockResolvedValue({ ...pedido({ outstanding: '175' }), lines: [] });

  await u.click(await screen.findByRole('button', { name: 'Efectivo' }));
  await u.type(screen.getByLabelText('Con cuánto paga'), '50');

  await waitFor(() => {
    expect(screen.getByRole('button', { name: /^Cobrar / })).toBeDisabled();
  });
});

// Un método desactivado sigue cobrando en el servidor (GetPaymentMethod no filtra is_active), así
// que el catálogo del front es la única barrera — y una tableta encendida lleva horas con él en
// caché.
test('vuelve a pedir el catálogo de métodos al abrir', async () => {
  pinta(<CobrarSheet order={pedido()} onClose={() => {}} onCobrado={() => {}} />);
  await waitFor(() => expect(paymentMethods).toHaveBeenCalled());
});

// Una plataforma sin métodos propios devuelve la lista vacía A PROPÓSITO: cobrar un pedido de Uber
// con el efectivo del mostrador hace que el corte espere en el cajón billetes que la plataforma
// pagó por transferencia. La fila en blanco dejaba al operador sin saber qué le faltaba.
test('sin métodos elegibles lo dice con palabras, no deja la fila vacía', async () => {
  paymentMethods.mockResolvedValue({ items: metodos });
  pinta(<CobrarSheet order={pedido({ deliveryPlatformId: 3 })} onClose={() => {}} onCobrado={() => {}} />);

  expect(await screen.findByText(/no tiene métodos de pago configurados/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /^Cobrar / })).toBeDisabled();
});

// "Quédese con el cambio" no tenía gesto: capturar el total recibido como monto rebota con
// ErrCobroExcede y dejaba al operador atorado con el cliente enfrente.
test('el cambio se puede dejar como propina de un toque', async () => {
  const u = userEvent.setup();
  order.mockResolvedValue({ ...pedido({ outstanding: '460', total: '460' }), lines: [] });
  pinta(<CobrarSheet order={pedido({ outstanding: '460', total: '460' })} onClose={() => {}} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: 'Efectivo' }));
  await u.click(await screen.findByRole('button', { name: '$500' }));

  const boton = await screen.findByRole('button', { name: 'El cambio es propina' });
  await u.click(boton);
  expect(await screen.findByRole('button', { name: /^Cobrar \$500/ })).toBeEnabled();
});

// La hoja se puede abrir sobre un pedido que otra caja acaba de saldar. Decir "escribe cuánto vas a
// cobrar" ahí manda al operador a buscar un problema que no existe.
test('sobre un pedido ya saldado lo dice y no ofrece cobrar', async () => {
  order.mockResolvedValue({ ...pedido({ outstanding: '0', paid: true }), lines: [] });
  pinta(<CobrarSheet order={pedido({ outstanding: '0', paid: true })} onClose={() => {}} onCobrado={() => {}} />);

  expect(await screen.findByText('Este pedido ya está cobrado.')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /^Cobrar / })).toBeNull();
  expect(screen.getByRole('button', { name: 'Cerrar' })).toBeInTheDocument();
});
