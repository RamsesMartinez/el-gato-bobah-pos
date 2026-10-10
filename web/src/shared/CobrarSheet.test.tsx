import { vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';
import type { BoardOrder } from '../types/pos';
import { round2 } from '../domain/cobro';

const order = vi.hoisted(() => vi.fn());
const paymentMethods = vi.hoisted(() => vi.fn());
const chargeOrder = vi.hoisted(() => vi.fn());
const setOrderDiscount = vi.hoisted(() => vi.fn());
const chargeOrderShape = vi.hoisted(() => vi.fn());
const quoteOrder = vi.hoisted(() => vi.fn());
const cardTerminals = vi.hoisted(() => vi.fn());
const defaultTerminal = vi.hoisted(() => vi.fn());
vi.mock('../api/pos', () => ({
  posApi: {
    order, paymentMethods, chargeOrder, setOrderDiscount, chargeOrderShape, quoteOrder,
    cardTerminals, defaultTerminal,
    businessSettings: () => Promise.resolve({ timezone: 'America/Mexico_City' }),
  },
}));

// La cotización de una parte como la calcula el servidor: lo que falta entre las partes que quedan,
// con el residuo en la última. Vive en el mock porque la hoja ya no calcula partes.
function partOf(outstanding: number, of: number, charged: number) {
  const left = of - charged;
  if (left === 1) return outstanding;
  return Math.floor((outstanding * 100) / left) / 100;
}

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
  setOrderDiscount.mockReset();
  chargeOrderShape.mockReset();
  quoteOrder.mockReset();
  cardTerminals.mockResolvedValue({ items: [{ id: 5, branchId: 1, branchName: 'Matriz', name: 'Getnet', archived: false }] });
  defaultTerminal.mockResolvedValue(null);
  // Por omisión cotiza una parte de N sobre el faltante del pedido de prueba, sin partes cobradas.
  quoteOrder.mockImplementation(async (_id: number, shape: { split?: { part: number; of: number } }) => ({
    amount: String(partOf(500, shape.split?.of ?? 2, (shape.split?.part ?? 1) - 1)), lines: [], outstandingAfter: '0',
  }));
});

// La hoja tiene que decir las DOS cifras. Pintando solo el faltante donde el operador espera el
// total, un pedido de $500 con $300 ya abonados se ve idéntico a uno de $200 y nadie puede notar la
// diferencia desde esta pantalla.
test('el encabezado dice el total del pedido y lo que falta, no solo una', async () => {
  order.mockResolvedValue({ ...pedido({ outstanding: '200' }), lines: [] });
  pinta(<CobrarSheet pantalla="pos" order={pedido({ outstanding: '200' })} onClose={() => {}} onCobrado={() => {}} />);

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
  pinta(<CobrarSheet pantalla="pos" order={pedido({ outstanding: '500' })} onClose={() => {}} onCobrado={() => {}} />);

  expect(await screen.findByText(/Falta \$120/)).toBeInTheDocument();
});

// NINGÚN MÉTODO VIENE PRESELECCIONADO, y no es una omisión.
//
// Aquí el pedido ya existe: un dedo que va directo a Cobrar con un método puesto por default
// registra con tarjeta dinero que entró en efectivo, y el corte cierra descuadrado en los dos
// métodos a la vez. El tap sobre el método es la confirmación de con qué se está pagando.
test('no se puede cobrar sin elegir método', async () => {
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  const boton = await screen.findByRole('button', { name: /^Cobrar / });
  expect(boton).toBeDisabled();
  expect(screen.getByText('Falta con qué paga.')).toBeInTheDocument();
});

// LA HOJA ABRE PARA COBRARLE A UNA SOLA PERSONA, que es como se cobra casi siempre.
//
// Antes traía cuatro botones fijos —Todo, entre 2, entre 3, entre 4— siempre en pantalla. En una
// hoja donde lo que escasea es el ALTO, eso es una fila entera gastada en el caso raro: el operador
// veía el repartidor en cada cobro y lo usaba en uno de cada varias decenas.
test('abre sin repartidor: el monto es todo lo que falta', async () => {
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  expect(await screen.findByRole('button', { name: /^Cobrar \$500/ })).toBeInTheDocument();
  // Ni el repartidor ni sus controles ocupan nada hasta que alguien los pide.
  expect(screen.queryByLabelText('Una parte más')).toBeNull();
  expect(screen.queryByLabelText('Otro monto')).toBeNull();
});

// REPARTIR ES DINÁMICO: el número de partes lo pone el operador, no una lista de cuatro.
//
// Una mesa de seis es tan común como una de tres, y con los presets fijos había que teclear el
// monto — con el teclado del sistema comiéndose 250 de los 600 px de alto y tapando la cifra que
// decide si el botón se enciende.
test('al dividir entre personas, el número sube y baja y el monto lo cotiza el servidor', async () => {
  const u = userEvent.setup();
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: /Dividir/ }));
  // Sin productos que elegir, abre entre personas, en dos, que es el reparto más común.
  expect(screen.getByText('2 personas')).toBeInTheDocument();
  expect(await screen.findByRole('button', { name: /^Cobrar \$250/ })).toBeInTheDocument();
  expect(quoteOrder).toHaveBeenLastCalledWith(7, { split: { part: 1, of: 2 } });

  await u.click(screen.getByLabelText('Una persona más'));
  expect(screen.getByText('3 personas')).toBeInTheDocument();
  expect(await screen.findByRole('button', { name: /^Cobrar \$166\.66/ })).toBeInTheDocument();

  await u.click(screen.getByLabelText('Una persona menos'));
  expect(await screen.findByRole('button', { name: /^Cobrar \$250/ })).toBeInTheDocument();

  // Y se puede volver a cobrar todo junto de un toque, sin cerrar la hoja.
  await u.click(screen.getByRole('button', { name: 'Dejar de dividir' }));
  expect(screen.getByRole('button', { name: /^Cobrar \$500/ })).toBeInTheDocument();
  expect(screen.queryByLabelText('Una persona más')).toBeNull();
});

// El repartidor no puede ofrecer lo que el cobro va a rechazar: con $0.02 pendientes, tres partes
// serían de $0.00. El `+` se apaga en vez de dejar el botón muerto sin decir por qué.
test('no deja repartir en más personas que el tope', async () => {
  const u = userEvent.setup();
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: /Dividir/ }));
  for (let i = 2; i < 12; i++) await u.click(screen.getByLabelText('Una persona más'));
  expect(screen.getByText('12 personas')).toBeInTheDocument();
  expect(screen.getByLabelText('Una persona más')).toBeDisabled();
});

// UN COBRO A LA VEZ, con su llave. Mandar N pagos de un golpe registra dinero que todavía no se
// recibió, y no existe forma de deshacer un pago: no hay endpoint que lo quite y el reembolso es de
// la cuenta entera.
test('cobrar una parte manda UNA llamada con la parte, sin monto, y con su llave', async () => {
  const u = userEvent.setup();
  chargeOrderShape.mockResolvedValue({ outstanding: '250', paid: false, yaEstaba: false, amount: '250' });
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: /Dividir/ }));
  await u.click(screen.getByRole('button', { name: 'Tarjeta' }));
  await u.click(await screen.findByRole('button', { name: /^Cobrar \$250/ }));

  await waitFor(() => expect(chargeOrderShape).toHaveBeenCalledTimes(1));
  const [id, body] = chargeOrderShape.mock.calls[0];
  expect(id).toBe(7);
  // El monto lo calcula el servidor: mandarlo sería pedirle que respete una segunda regla.
  expect(body).not.toHaveProperty('amount');
  expect(body.split).toEqual({ part: 1, of: 2 });
  expect(body.methodId).toBe(2);
  expect(typeof body.clientUuid).toBe('string');
  expect(chargeOrder).not.toHaveBeenCalled();
});

// La hoja NO se cierra mientras quede saldo: el siguiente comensal todavía tiene que pagar, y
// cerrarla obligaría a volver a buscar el pedido en la lista con la mesa esperando.
test('con saldo pendiente la hoja sigue abierta y el pago queda a la vista desde el servidor', async () => {
  const u = userEvent.setup();
  const onClose = vi.fn();
  chargeOrderShape.mockResolvedValue({ outstanding: '250', paid: false, yaEstaba: false, amount: '250' });
  order.mockResolvedValueOnce({ ...pedido(), lines: [] });
  order.mockResolvedValue({
    ...pedido({ outstanding: '250' }), lines: [],
    payments: [{ id: 1, number: 1, voided: false, methodId: 2, methodName: 'Tarjeta', amount: '250', tip: '0',
      reference: '', paidAt: '', receivedBy: '', split: { part: 1, of: 2 }, lines: [] }],
  });
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={onClose} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: /Dividir/ }));
  await u.click(screen.getByRole('button', { name: 'Tarjeta' }));
  await u.click(await screen.findByRole('button', { name: /^Cobrar \$250/ }));

  await waitFor(() => expect(chargeOrderShape).toHaveBeenCalled());
  expect(onClose).not.toHaveBeenCalled();
  // Lo que ya entró sale de los pagos del pedido, no de la memoria de la hoja: sobrevive a recargar.
  expect(await screen.findByText(/Pago 1 · Tarjeta · \$250/)).toBeInTheDocument();
});

// Saldado el pedido sí se cierra: dejarla abierta sobre algo que ya no debe nada invita a cobrarlo
// otra vez.
test('al quedar saldado se cierra', async () => {
  const u = userEvent.setup();
  const onClose = vi.fn();
  chargeOrder.mockResolvedValue({ outstanding: '0', paid: true, yaEstaba: false });
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={onClose} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: 'Tarjeta' }));
  await u.click(screen.getByRole('button', { name: /^Cobrar \$500/ }));

  await waitFor(() => expect(onClose).toHaveBeenCalled());
});

// EL CASO CARO: el operador tiene el efectivo del cliente en la mano y el servidor dice que no.
// `String(e)` ahí es un objeto de error crudo en la pantalla de quien tiene que decidir qué hacer.
test('traduce el rebote de otra caja a algo accionable', async () => {
  const u = userEvent.setup();
  chargeOrder.mockRejectedValue(new Error('ese pedido ya está cobrado'));
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: 'Tarjeta' }));
  await u.click(screen.getByRole('button', { name: /^Cobrar \$500/ }));

  expect(await screen.findByText('Otra caja acaba de cobrar este pedido')).toBeInTheDocument();
});

// En efectivo, el aviso de faltante es el único control que impide cobrar de menos. En modo
// dividido no existía en ninguna de las dos hojas.
test('en efectivo no deja cobrar si lo recibido no alcanza', async () => {
  const u = userEvent.setup();
  pinta(<CobrarSheet pantalla="pos" order={pedido({ outstanding: '175' })} onClose={() => {}} onCobrado={() => {}} />);
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
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);
  await waitFor(() => expect(paymentMethods).toHaveBeenCalled());
});

// Una plataforma sin métodos propios devuelve la lista vacía A PROPÓSITO: cobrar un pedido de Uber
// con el efectivo del mostrador hace que el corte espere en el cajón billetes que la plataforma
// pagó por transferencia. La fila en blanco dejaba al operador sin saber qué le faltaba.
test('sin métodos elegibles lo dice con palabras, no deja la fila vacía', async () => {
  paymentMethods.mockResolvedValue({ items: metodos });
  pinta(<CobrarSheet pantalla="pos" order={pedido({ deliveryPlatformId: 3 })} onClose={() => {}} onCobrado={() => {}} />);

  expect(await screen.findByText(/no tiene métodos de pago configurados/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /^Cobrar / })).toBeDisabled();
});

// "Quédese con el cambio" no tenía gesto: capturar el total recibido como monto rebota con
// ErrCobroExcede y dejaba al operador atorado con el cliente enfrente.
test('el cambio se puede dejar como propina de un toque', async () => {
  const u = userEvent.setup();
  order.mockResolvedValue({ ...pedido({ outstanding: '460', total: '460' }), lines: [] });
  pinta(<CobrarSheet pantalla="pos" order={pedido({ outstanding: '460', total: '460' })} onClose={() => {}} onCobrado={() => {}} />);

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
  pinta(<CobrarSheet pantalla="pos" order={pedido({ outstanding: '0', paid: true })} onClose={() => {}} onCobrado={() => {}} />);

  expect(await screen.findByText('Este pedido ya está cobrado.')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /^Cobrar / })).toBeNull();
  expect(screen.getByRole('button', { name: 'Cerrar' })).toBeInTheDocument();
});

// EL CASO PARA EL QUE EXISTE LA LLAVE, y el que se rompía generándola en cada envío.
//
// El cobro entra, la respuesta se pierde en la red, el operador vuelve a tocar. Con llave nueva el
// servidor no tiene cómo saber que es el mismo cobro y lo registra otra vez: el cliente paga dos
// veces y el corte cierra con un sobrante que nadie sabe de dónde salió.
test('el reintento de un cobro fallido manda la MISMA llave', async () => {
  const u = userEvent.setup();
  chargeOrder.mockRejectedValueOnce(new Error('network error'));
  chargeOrder.mockResolvedValueOnce({ outstanding: '0', paid: true, yaEstaba: true });
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: 'Tarjeta' }));
  await u.click(screen.getByRole('button', { name: /^Cobrar \$500/ }));
  await screen.findByText('No se pudo cobrar');

  await u.click(screen.getByRole('button', { name: /^Cobrar \$500/ }));
  await waitFor(() => expect(chargeOrder).toHaveBeenCalledTimes(2));

  const primera = chargeOrder.mock.calls[0][1].clientUuid;
  const segunda = chargeOrder.mock.calls[1][1].clientUuid;
  expect(segunda, 'el reintento cambió de llave: el servidor lo cobraría dos veces').toBe(primera);
});

// Y al cobrar un pedazo con éxito, el siguiente comensal va con llave NUEVA: es otro cobro, y
// reusarla lo haría rebotar como reintento de uno que ya entró.
test('cada parte cobrada estrena llave', async () => {
  const u = userEvent.setup();
  chargeOrderShape.mockResolvedValue({ outstanding: '250', paid: false, yaEstaba: false, amount: '250' });
  order.mockResolvedValueOnce({ ...pedido(), lines: [] });
  order.mockResolvedValue({
    ...pedido({ outstanding: '250' }), lines: [],
    payments: [{ id: 1, number: 1, voided: false, methodId: 2, methodName: 'Tarjeta', amount: '250', tip: '0',
      reference: '', paidAt: '', receivedBy: '', split: { part: 1, of: 2 }, lines: [] }],
  });
  quoteOrder.mockResolvedValue({ amount: '250', lines: [], outstandingAfter: '0' });
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: /Dividir/ }));
  await u.click(screen.getByRole('button', { name: 'Tarjeta' }));
  await u.click(await screen.findByRole('button', { name: /^Cobrar \$250/ }));
  await waitFor(() => expect(chargeOrderShape).toHaveBeenCalledTimes(1));

  await u.click(await screen.findByRole('button', { name: 'Efectivo' }));
  await waitFor(() => expect(screen.getByRole('button', { name: /^Cobrar \$250/ })).toBeEnabled());
  await u.click(screen.getByRole('button', { name: /^Cobrar \$250/ }));
  await waitFor(() => expect(chargeOrderShape).toHaveBeenCalledTimes(2));

  expect(chargeOrderShape.mock.calls[1][1].clientUuid).not.toBe(chargeOrderShape.mock.calls[0][1].clientUuid);
  // La segunda va por la parte que sigue, que la hoja saca de los pagos del servidor.
  expect(chargeOrderShape.mock.calls[1][1].split).toEqual({ part: 2, of: 2 });
});

// REPARTIR DE A UNO TIENE QUE CERRAR LA CUENTA EXACTA.
//
// Cada parte se recalcula sobre el faltante que devuelve el SERVIDOR, no sobre una lista hecha al
// abrir la hoja: entre un pedazo y otro el faltante puede cambiar. El riesgo del recálculo es el
// centavo colgando —$100 en tres da tres de $33.33 y suma $99.99—, y ese centavo ya costó: el
// servidor cerraba el pedido con su tolerancia y la barra lo seguía listando como deuda que nadie
// podía cobrar.
test('repartir entre tres cobra las partes 1, 2 y 3 y el servidor pone los montos', async () => {
  const u = userEvent.setup();
  const cien = pedido({ total: '100', outstanding: '100' });
  // El servidor lleva lo que falta y las partes cobradas; la hoja lee de ahí, nunca de una resta.
  let faltante = 100;
  const pagos: Array<Record<string, unknown>> = [];
  order.mockImplementation(async () => ({ ...cien, outstanding: String(faltante), lines: [], payments: [...pagos] }));
  quoteOrder.mockImplementation(async (_id: number, shape: { split: { part: number; of: number } }) =>
    ({ amount: String(partOf(faltante, shape.split.of, pagos.length)), lines: [], outstandingAfter: '0' }));
  chargeOrderShape.mockImplementation(async (_id: number, body: { split: { part: number; of: number }; methodId: number }) => {
    const amount = partOf(faltante, body.split.of, pagos.length);
    faltante = round2(faltante - amount);
    pagos.push({ id: pagos.length + 1, number: pagos.length + 1, voided: false, methodId: body.methodId,
      methodName: body.methodId === 1 ? 'Efectivo' : 'Tarjeta', amount: String(amount), tip: '0', reference: '',
      paidAt: '', receivedBy: '', split: body.split, lines: [] });
    return { outstanding: String(faltante), paid: faltante <= 0, yaEstaba: false, amount: String(amount) };
  });
  pinta(<CobrarSheet pantalla="pos" order={cien} onClose={() => {}} onCobrado={() => {}} />);

  await u.click(await screen.findByRole('button', { name: /Dividir/ }));
  await u.click(screen.getByLabelText('Una persona más'));
  expect(screen.getByText('3 personas')).toBeInTheDocument();

  const cobrarCon = async (metodo: string, monto: RegExp) => {
    await u.click(screen.getByRole('button', { name: metodo }));
    await u.click(await screen.findByRole('button', { name: monto }));
  };
  await cobrarCon('Tarjeta', /^Cobrar \$33\.33/);
  // Cada persona paga con LO SUYO: el método no se hereda del pedazo anterior.
  await waitFor(() => expect(screen.getByRole('button', { name: /^Cobrar \$/ })).toBeDisabled());
  await cobrarCon('Efectivo', /^Cobrar \$33\.33/);
  await cobrarCon('Tarjeta', /^Cobrar \$33\.34/);

  await waitFor(() => expect(chargeOrderShape).toHaveBeenCalledTimes(3));
  expect(chargeOrderShape.mock.calls.map((c) => c[1].split.part)).toEqual([1, 2, 3]);
});

// UNA SOLA PUERTA PARA COBRAR (spec 030, US4).
//
// La hoja se abre SIEMPRE sobre un pedido: si la cuenta tenía algo sin enviar, «Enviar y cobrar» lo
// manda primero (research R-5). Así «Por productos» —que necesita los renglones del pedido— sale
// desde el ticket igual que desde cualquier otro lado.
test('una cuenta de mostrador ofrece los tres modos al dividir', async () => {
  order.mockResolvedValue({
    ...pedido(), canSplit: true,
    lines: [{ id: 1, productName: 'Taro', quantity: '2', unitPrice: '250', lineTotal: '500', delivered: '0', cancelled: false }],
    payments: [],
  });
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);
  await userEvent.click(await screen.findByRole('button', { name: 'Dividir' }));
  for (const m of ['Por productos', 'Entre personas', 'Por monto']) {
    expect(await screen.findByRole('button', { name: m })).toBeInTheDocument();
  }
});

// FR-008 / D-11: un pedido de plataforma se cobra completo con el método de SU plataforma. Ni se
// divide entre personas ni por productos: la plataforma ya lo cobró entero.
test('un pedido de plataforma no se divide: solo se cobra completo', async () => {
  paymentMethods.mockResolvedValue({ items: [...metodos, { id: 9, name: 'Uber Eats', kind: 'plataforma', deliveryPlatformId: 3 }] });
  order.mockResolvedValue({ ...pedido({ deliveryPlatformId: 3 }), lines: [], payments: [], canSplit: false });
  pinta(<CobrarSheet pantalla="pos" order={pedido({ deliveryPlatformId: 3 })} onClose={() => {}} onCobrado={() => {}} />);
  expect(await screen.findByRole('button', { name: 'Uber Eats' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Efectivo' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Dividir' })).toBeNull();
});

// US4 AS2 / caso 11: cerrar la hoja sin cobrar no cancela nada. La cuenta sigue en la fila tal
// como quedó (en cocina, si «Enviar y cobrar» la mandó).
test('cerrar la hoja sin cobrar solo la cierra: no cobra ni cancela nada', async () => {
  const onClose = vi.fn();
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={onClose} onCobrado={() => {}} />);
  await screen.findByRole('button', { name: 'Efectivo' });
  await userEvent.keyboard('{Escape}');
  await waitFor(() => expect(onClose).toHaveBeenCalled());
  expect(chargeOrder).not.toHaveBeenCalled();
  expect(chargeOrderShape).not.toHaveBeenCalled();
});

// ---------------------------------------------------------------------------------------------
// CORREGIR EL DESCUENTO DE UN PEDIDO YA CREADO (023 / la T031 que la 022 dejó abierta).
//
// El descuento se teclea con el cliente enfrente y se teclea mal. El momento en que el error se
// descubre es este: se va a cobrar, se mira el total y no cuadra.
// ---------------------------------------------------------------------------------------------

test('se puede corregir el descuento y lo que se pinta es lo que devolvió el servidor', async () => {
  const conDescuentoDe50 = { ...pedido({ total: '450', outstanding: '450' }), discount: '50', lines: [] };
  const conDescuentoDe30 = { ...pedido({ total: '470', outstanding: '470' }), discount: '30', lines: [] };
  order.mockResolvedValue(conDescuentoDe50);
  // El servidor devuelve el pedido ya recalculado Y las lecturas posteriores ven lo mismo: la hoja
  // invalida la familia `orders` a propósito, para que el tablero y la barra se enteren.
  setOrderDiscount.mockImplementation(async () => {
    order.mockResolvedValue(conDescuentoDe30);
    return conDescuentoDe30;
  });
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await userEvent.click(await screen.findByRole('button', { name: /Cambiar el descuento/ }));
  const campo = await screen.findByLabelText('Descuento');
  await userEvent.clear(campo);
  await userEvent.type(campo, '30');
  await userEvent.click(screen.getByRole('button', { name: 'Guardar' }));

  await waitFor(() => expect(setOrderDiscount).toHaveBeenCalledWith(7, { discountAmount: 30 }));
  // Lo que se pinta después es la respuesta del servidor, no una resta hecha aquí: una segunda
  // implementación de la misma cifra es como la pantalla llegó a ofrecer cobrar $115 de un pedido
  // de $95.
  expect(await screen.findByText(/Falta \$470/)).toBeInTheDocument();
});

// Un pedido ya cobrado no admite mover su total: el servidor lo rechaza y la pantalla lo dice sin
// inventar nada.
test('si el servidor rechaza el cambio, ninguna cifra se mueve', async () => {
  order.mockResolvedValue({ ...pedido({ total: '450', outstanding: '0', paid: true }), discount: '50', lines: [] });
  setOrderDiscount.mockRejectedValue(new Error('el pedido ya está cobrado'));
  pinta(<CobrarSheet pantalla="pos" order={pedido({ paid: true, outstanding: '0' })} onClose={() => {}} onCobrado={() => {}} />);

  await userEvent.click(await screen.findByRole('button', { name: /Cambiar el descuento/ }));
  await userEvent.type(await screen.findByLabelText('Descuento'), '10');
  await userEvent.click(screen.getByRole('button', { name: 'Guardar' }));

  expect(await screen.findByText(/ya está cobrado/)).toBeInTheDocument();
  expect(screen.getByText(/Total \$450/), 'la cifra se movió con un cambio que el servidor rechazó')
    .toBeInTheDocument();
});

// EL DEFECTO QUE ESTE TEST CIERRA: `{}` no significa "sin cambios" para el servidor, significa
// "quita el descuento". Y un campo ilegible producía exactamente ese `{}`.
//
// Un pedido con $50 de descuento, una letra de más al corregir, un toque en Guardar, y el descuento
// desaparecía: sin aviso, sin confirmación, y sin más rastro que un "Falta" que subió $50.
test('un descuento mal tecleado no puede borrar el que ya estaba', async () => {
  order.mockResolvedValue({ ...pedido({ total: '450', outstanding: '450' }), discount: '50', lines: [] });
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await userEvent.click(await screen.findByRole('button', { name: /Cambiar el descuento/ }));
  const campo = await screen.findByLabelText('Descuento');
  await userEvent.clear(campo);
  await userEvent.type(campo, '1,000');

  expect(await screen.findByText('Solo números')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Guardar' }),
    'Guardar sigue vivo con un valor ilegible: al tocarlo manda {} y el servidor BORRA el descuento')
    .toBeDisabled();
  expect(setOrderDiscount).not.toHaveBeenCalled();
});

// Lo mismo con un monto imposible: el tope se avisa y no se manda nada.
test('un descuento mayor que la cuenta tampoco se puede guardar', async () => {
  order.mockResolvedValue({ ...pedido({ total: '450', outstanding: '450' }), discount: '50', lines: [] });
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await userEvent.click(await screen.findByRole('button', { name: /Cambiar el descuento/ }));
  const campo = await screen.findByLabelText('Descuento');
  await userEvent.clear(campo);
  await userEvent.type(campo, '9000');

  expect(await screen.findByText(/Máx/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
});

// Un renglón que siempre dice $0.00 enseña a no leer esta zona, que es donde vive el dinero.
test('un pedido sin descuento no muestra un renglón en cero, pero deja agregar uno', async () => {
  order.mockResolvedValue({ ...pedido(), discount: '0', lines: [] });
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);

  await screen.findByText(/Falta \$500/);
  expect(screen.queryByText(/\$0\.00 de descuento/)).toBeNull();
  // El acceso vive en el encabezado, junto a «Cuenta» y «Dividir»: ahí no cuesta alto nuevo, y el
  // flujo más común de esta hoja —cobrar un pedido ya mandado— casi nunca lleva descuento.
  expect(screen.getByRole('button', { name: /Descuento/ })).toBeInTheDocument();
});

// CON TARJETA SE REGISTRA LA TERMINAL (spec 032, punto 8). Con una sola, va sola: el caso común no
// cuesta un toque más.
test('cobrar con tarjeta manda la terminal que llegó puesta', async () => {
  chargeOrder.mockResolvedValue({ outstanding: '0', paid: true, yaEstaba: false, amount: '500' });
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);
  await userEvent.click(await screen.findByRole('button', { name: 'Tarjeta' }));
  expect(await screen.findByRole('button', { name: /Getnet/ })).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: /^Cobrar / }));
  await waitFor(() => expect(chargeOrder).toHaveBeenCalledTimes(1));
  expect(chargeOrder.mock.calls[0][1]).toMatchObject({ methodId: 2, terminalId: 5 });
});

// Con varias terminales y sin la del usuario, se pide antes de cobrar (EB-31).
test('con varias terminales y ninguna puesta no deja cobrar con tarjeta', async () => {
  cardTerminals.mockResolvedValue({ items: [
    { id: 5, branchId: 1, branchName: 'Matriz', name: 'Getnet', archived: false },
    { id: 6, branchId: 1, branchName: 'Matriz', name: 'Hey', archived: false },
  ] });
  pinta(<CobrarSheet pantalla="pos" order={pedido()} onClose={() => {}} onCobrado={() => {}} />);
  await userEvent.click(await screen.findByRole('button', { name: 'Tarjeta' }));
  expect(await screen.findByRole('button', { name: /Elige la terminal/ })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /^Cobrar / })).toBeDisabled();
});
