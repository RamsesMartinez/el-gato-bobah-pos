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
  envio: '',
  onEnvio: vi.fn(),
  envioPorDefecto: 20,
  noDisponibles: [],
};

// El renglón del producto también pinta su precio: el total se busca por su etiqueta, no por la
// cifra suelta.
async function totalEnPantalla() {
  const etiqueta = await screen.findByText('Total');
  return etiqueta.parentElement?.querySelector('p:last-child')?.textContent
    ?? etiqueta.nextElementSibling?.textContent;
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
  const { rerender } = pinta(<Ticket {...props} envio="1,000" />);

  expect(await screen.findByText('Solo números')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'COBRAR' })).toBeDisabled();
  expect(screen.getByRole('button', { name: /Enviar a cocina/ })).toBeDisabled();

  // Y bien escrito sí deja seguir: la regla rechaza el formato, no el número.
  rerender(<ChakraProvider value={defaultSystem}><Ticket {...props} envio="30" /></ChakraProvider>);
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
  pinta(<Ticket {...props} envio="20" />);

  expect(screen.queryByLabelText('Costo de envío')).toBeNull();
  // Y el total no lo incluye: el reparto lo cobra la plataforma.
  expect(await totalEnPantalla()).toBe('$95');
});

// El envío del domicilio propio SÍ entra al total que ve el operador, o el ticket dice una cifra y
// el cobro otra.
test('un domicilio propio suma el envío al total de la pantalla', async () => {
  useTicketStore.getState().setServiceType('domicilio');
  pinta(<Ticket {...props} envio="30" />);
  expect(await totalEnPantalla()).toBe('$125');
});

// Sin capturar nada, el envío es el del negocio: el campo vacío significa "el de siempre", no cero.
test('sin capturar envío se usa el del negocio', async () => {
  useTicketStore.getState().setServiceType('domicilio');
  pinta(<Ticket {...props} envio="" />);
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
