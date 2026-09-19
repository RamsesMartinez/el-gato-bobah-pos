import { vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';

import { Ticket } from './Ticket';
import { useTicketStore } from '../../stores/ticket';

function pinta(nodo: ReactNode) {
  return render(<ChakraProvider value={defaultSystem}>{nodo}</ChakraProvider>);
}

const props = {
  onCheckout: vi.fn(),
  onEnviar: vi.fn(),
  enviando: false,
  onEditLine: vi.fn(),
  onHide: vi.fn(),
  envioPorDefecto: 20,
  noDisponibles: [],
};

// El renglón del producto también pinta su precio: el total se busca por su etiqueta, no por la
// cifra suelta. La etiqueta "Total" comparte grupo con el botón de descuento, y la cifra es el
// último texto de esa fila —debajo puede ir el renglón chico que explica el descuento—.
async function totalEnPantalla() {
  const etiqueta = await screen.findByText('Total');
  const fila = etiqueta.closest('div')?.parentElement;
  const textos = fila?.querySelectorAll('p');
  return textos?.[textos.length - 1]?.textContent;
}

beforeEach(() => {
  vi.clearAllMocks();
  // Cada test arranca con una cuenta limpia y un solo producto de $95: el panel muestra el total de
  // la cuenta activa, así que arrastrar renglones entre tests haría que la cifra dependa del orden.
  useTicketStore.setState(useTicketStore.getInitialState(), true);
  useTicketStore.getState().addLine({ productId: 1, name: 'Alitas', unitPrice: 95, qty: 1, modifiers: [] });
});

// EL DEFECTO QUE ESTO CIERRA: un costo de envío mal escrito se convertía en ENVÍO GRATIS.
//
// `parseFloat('1,000') || 0` daba 1, y cualquier cosa que no empezara con un dígito daba 0. El
// renglón desaparecía del total, el pedido se creaba sin envío, y nadie se enteraba hasta cuadrar
// la caja. El default es para el campo AUSENTE, nunca para el presente y malformado.
test('un envío mal escrito no cobra envío gratis: apaga los botones y lo dice', async () => {
  useTicketStore.getState().setServiceType('domicilio');
  useTicketStore.getState().setEnvio('1,000');
  const { rerender } = pinta(<Ticket {...props} />);

  expect(await screen.findByText('Solo números')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'COBRAR' })).toBeDisabled();
  expect(screen.getByRole('button', { name: /Enviar a cocina/ })).toBeDisabled();

  // Y bien escrito sí deja seguir: la regla rechaza el formato, no el número.
  useTicketStore.getState().setEnvio('30');
  rerender(<ChakraProvider value={defaultSystem}><Ticket {...props} /></ChakraProvider>);
  expect(screen.queryByText('Solo números')).toBeNull();
  expect(screen.getByRole('button', { name: 'COBRAR' })).toBeEnabled();
});

// EL DEFECTO: la pantalla ofrecía cobrar un envío que el servidor no cobra.
//
// Se marca "Domicilio", después se asigna la plataforma, y el panel esconde los botones de tipo:
// la cuenta queda en domicilio con plataforma y el operador ya no puede corregirlo. Sumaba $20 que
// el servidor fuerza a 0, y el cobro rebotaba dejando el pedido creado y sin cobrar.
test('con plataforma no se ofrece envío, aunque la cuenta diga domicilio', async () => {
  useTicketStore.getState().setServiceType('domicilio');
  useTicketStore.getState().setPlatform(3);
  pinta(<Ticket {...props} />);

  expect(screen.queryByLabelText('Costo de envío')).toBeNull();
  // Y el total no lo incluye: el reparto lo cobra la plataforma.
  expect(await totalEnPantalla()).toBe('$95');
});

// El envío del domicilio propio SÍ entra al total que ve el operador, o el ticket dice una cifra y
// el cobro otra.
test('un domicilio propio suma el envío al total de la pantalla', async () => {
  useTicketStore.getState().setServiceType('domicilio');
  useTicketStore.getState().setEnvio('30');
  pinta(<Ticket {...props} />);
  expect(await totalEnPantalla()).toBe('$125');
});

// Sin capturar nada, el envío es el del negocio: el campo vacío significa "el de siempre", no cero.
test('sin capturar envío se usa el del negocio', async () => {
  useTicketStore.getState().setServiceType('domicilio');
  pinta(<Ticket {...props} />);
  expect(await totalEnPantalla()).toBe('$115');
});

// EL DEFECTO: enterarse al COBRAR de que un producto ya no está en el menú.
//
// El aviso vivía en la hoja de cobro, así que el operador lo descubría con el cliente enfrente y el
// dinero en la mano. Aquí se ve mientras la cuenta se arma y se quita de un toque.
test('avisa de los productos que ya no están en el menú, mientras se puede quitar', async () => {
  const u = userEvent.setup();
  const fuera = { lineId: 'x', productId: 9, name: 'Tamarindo', unitPrice: 30, qty: 1, modifiers: [] };
  pinta(<Ticket {...props} noDisponibles={[fuera]} />);

  expect(await screen.findByText('Ya no están en el menú')).toBeInTheDocument();
  // Se NOMBRA el producto: con el carrito lleno, un aviso sin nombre no dice qué renglón quitar.
  expect(screen.getByText('Tamarindo')).toBeInTheDocument();
  await u.click(screen.getByRole('button', { name: /Quitar del pedido/ }));
});

// EL DEFECTO QUE EL DUEÑO REPORTÓ: "elijo elementos, le doy a Cobrar, y se eliminan; aunque cancele
// el modal ya no reaparecen".
//
// COBRAR dejó de ser "abre una pantalla que cobra al final": ahora CONFIRMA el pedido —lo manda a
// cocina— y después abre el cobro. Eso es lo correcto (cobrar sin que cocina se entere era el atajo
// que la feature 005 vino a cerrar), pero significa que el botón es irreversible desde el instante
// en que se toca, y la pantalla no lo decía. Quien cierra la hoja creyendo que canceló ve el carrito
// vacío, da la venta por perdida y la vuelve a capturar: cocina prepara dos veces lo mismo.
//
// El botón lo dice ahora. No es un aviso decorativo: es la diferencia entre un gesto reversible y
// uno que no lo es.
test('COBRAR avisa que también manda el pedido a cocina', async () => {
  pinta(<Ticket {...props} />);
  const cobrar = await screen.findByRole('button', { name: /COBRAR/ });
  expect(cobrar).toHaveAccessibleDescription(/cocina/i);
});

// UN ENVÍO QUE YA NO APLICA NO PUEDE DEJAR EL POS MUDO.
//
// El cálculo del envío corría siempre, pero el campo y el mensaje "Solo números" solo se pintaban
// en domicilio propio. Teclear "1,5" y luego cambiar a Mostrador —o asignar una plataforma— dejaba
// los dos botones apagados sin campo que corregir ni razón visible, y como el envío era global,
// ninguna cuenta podía vender.
test('un envío mal escrito deja de bloquear cuando la cuenta pasa a mostrador', async () => {
  useTicketStore.getState().setServiceType('domicilio');
  useTicketStore.getState().setEnvio('1,5');
  const { rerender } = pinta(<Ticket {...props} />);
  expect(screen.getByRole('button', { name: 'COBRAR' })).toBeDisabled();

  useTicketStore.getState().setServiceType('mostrador');
  rerender(<ChakraProvider value={defaultSystem}><Ticket {...props} /></ChakraProvider>);

  expect(await screen.findByRole('button', { name: 'COBRAR' })).toBeEnabled();
  // Y no queda un aviso huérfano de un campo que ya no se pinta.
  expect(screen.queryByText('Solo números')).toBeNull();
});

// El piso de 44 px se mide en Playwright y no aquí: las medidas de Chakra son clases CSS y jsdom no
// las resuelve, así que un assert de píxeles en este archivo pasaría verde con los botones de 24 px.
// Vive en e2e/cabe-en-la-tableta.spec.ts, contra un navegador real.

// La acción destructiva va SEPARADA de las frecuentes, no junto a ellas. Es requisito funcional:
// quitar un renglón por accidente obliga a volver a buscar el producto en el menú.
test('la papelera no está pegada a los botones de cantidad', () => {
  useTicketStore.getState().addLine({ productId: 1, name: 'Alitas', unitPrice: 95, qty: 1, modifiers: [] });
  pinta(<Ticket {...props} />);

  const menos = screen.getByRole('button', { name: '−' });
  const quitar = screen.getByRole('button', { name: 'Quitar' });
  // No comparten padre inmediato: el − vive con el + y la papelera se fue al otro extremo.
  expect(menos.parentElement, 'la papelera sigue pegada a los controles de cantidad')
    .not.toBe(quitar.parentElement);
});

// EL DESCUENTO, EN LA PANTALLA DE 600 px.
//
// El acceso vive en la fila del Total y no en una fila propia: una fila nueva le cobra ~52 px de
// alto a TODOS los pedidos —medido: baja de ~3.5 a ~2.9 los renglones de producto visibles en un
// domicilio— para servir al puñado que lleva promoción.
test('el descuento no ocupa alto hasta que alguien lo abre', async () => {
  pinta(<Ticket {...props} />);
  expect(screen.queryByLabelText('Descuento')).toBeNull();

  await userEvent.click(screen.getByRole('button', { name: 'Aplicar descuento' }));
  expect(await screen.findByLabelText('Descuento')).toBeInTheDocument();
});

test('un descuento en pesos baja el total y dice de dónde sale', async () => {
  useTicketStore.getState().setDescuento('20');
  pinta(<Ticket {...props} />);

  expect(await totalEnPantalla()).toBe('$75'); // 95 - 20
  // El renglón chico es lo que deja explicarle al cliente por qué el total no es la suma de los
  // renglones: sin él, la diferencia se discute en el mostrador.
  expect(screen.getByText(/de descuento/)).toBeInTheDocument();
});

test('un porcentaje se pinta en pesos', async () => {
  useTicketStore.getState().setDescuentoModo('pct');
  useTicketStore.getState().setDescuento('20');
  pinta(<Ticket {...props} />);

  expect(await totalEnPantalla()).toBe('$76'); // 95 - 19
});

// Un descuento ilegible que cayera a cero es una promoción que el cliente ya escuchó y que el
// ticket no aplica. Y uno mayor que la cuenta cobraría en negativo.
test('un descuento mal escrito o imposible apaga los botones', async () => {
  useTicketStore.getState().setDescuento('1,000');
  const { rerender } = pinta(<Ticket {...props} />);
  expect(await screen.findByText('Solo números')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'COBRAR' })).toBeDisabled();

  useTicketStore.getState().setDescuento('400'); // la cuenta son $95
  rerender(<ChakraProvider value={defaultSystem}><Ticket {...props} /></ChakraProvider>);
  expect(await screen.findByText(/Máx/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'COBRAR' })).toBeDisabled();
});

// FR-011: el descuento existe en los cuatro tipos de pedido. Hoy se cumple por omisión —no hay
// ninguna condición— y este test es lo que impide que una condición agregada después lo rompa sin
// que nada falle.
test.each([
  ['mostrador', null],
  ['domicilio', null],
  ['mostrador', 1],
  ['domicilio', 1],
] as const)('el descuento se puede capturar en %s con plataforma %s', async (tipo, plataforma) => {
  useTicketStore.getState().setServiceType(tipo);
  useTicketStore.getState().setPlatform(plataforma);
  pinta(<Ticket {...props} />);

  expect(screen.getByRole('button', { name: 'Aplicar descuento' })).toBeInTheDocument();
});

// `overflowY` sin un alto no hace scroll: la caja crece. En la tableta eso empuja el botón COBRAR
// fuera de la pantalla cuando el teclado numérico se abre sobre el campo de descuento, y no hay
// forma de alcanzarlo. Ningún test puede simular ese teclado; lo que sí se puede verificar es que
// la zona tenga alto acotado.
test('la zona de totales tiene alto acotado, no crece con lo que se le agregue', async () => {
  pinta(<Ticket {...props} />);
  const cobrar = await screen.findByRole('button', { name: 'COBRAR' });
  const zona = cobrar.closest('div')?.parentElement;
  expect(zona).not.toBeNull();
  const estilo = getComputedStyle(zona!);
  expect(estilo.maxHeight, 'sin maxH la caja crece y COBRAR se va abajo de la pantalla').not.toBe('none');
  expect(estilo.overflowY).toBe('auto');
});
