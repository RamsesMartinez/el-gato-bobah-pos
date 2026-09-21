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
  const fila = etiqueta.parentElement;
  const textos = fila?.querySelectorAll('p');
  return textos?.[textos.length - 1]?.textContent;
}

// Abre el campo del menú, que es donde ahora se captura. Los tests que solo necesitan el VALOR
// siguen escribiéndolo en el store; esto es para los que prueban el camino del dedo.
async function abrirDelMenu(opcion: RegExp) {
  await userEvent.click(await screen.findByRole('button', { name: 'Más opciones del pedido' }));
  await userEvent.click(await screen.findByRole('menuitem', { name: opcion }));
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

  await abrirDelMenu(/Descuento/);
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

  await abrirDelMenu(/Descuento/);
  expect(await screen.findByLabelText('Descuento')).toBeInTheDocument();
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

// ---------------------------------------------------------------------------------------------
// EL REACOMODO (023): arriba lo que describe el pedido, abajo lo que mueve dinero.
// ---------------------------------------------------------------------------------------------

function ponerFolio(nombre: string) {
  useTicketStore.setState((s) => ({ tabs: s.tabs.map((t) => ({ ...t, folioName: nombre })) }));
}

// La fila de tipo + cliente costaba 52 px en TODO pedido de mostrador, que es el de todos los días.
test('la zona de totales ya no trae el tipo ni el campo de cliente', async () => {
  pinta(<Ticket {...props} />);
  await screen.findByRole('button', { name: 'COBRAR' });

  // El tipo sigue existiendo, pero arriba: junto al nombre del pedido, no pegado a COBRAR.
  const tipo = screen.getByRole('button', { name: /Mostrador/ });
  const cobrar = screen.getByRole('button', { name: 'COBRAR' });
  expect(tipo.compareDocumentPosition(cobrar) & Node.DOCUMENT_POSITION_FOLLOWING,
    'el tipo quedó DESPUÉS de COBRAR: sigue en la zona del dinero').toBeTruthy();

  // Y el campo de cliente ya no ocupa alto: vive en el menú.
  expect(screen.queryByPlaceholderText('Cliente')).toBeNull();
});

// Un pedido de plataforma ES a domicilio: ofrecer el cambio sería ofrecer algo que el servidor
// rechaza por el check de la tabla.
test('un pedido de plataforma no ofrece cambiar el tipo', async () => {
  useTicketStore.getState().setPlatform(1);
  pinta(<Ticket {...props} />);
  await screen.findByRole('button', { name: 'COBRAR' });

  expect(screen.queryByRole('button', { name: /Mostrador|Domicilio/ })).toBeNull();
});

test('el menú del pedido ofrece cliente, descuento y vaciar', async () => {
  pinta(<Ticket {...props} />);
  await userEvent.click(await screen.findByRole('button', { name: 'Más opciones del pedido' }));

  for (const nombre of [/Nombre del cliente/, /Descuento/, /Vaciar/]) {
    expect(await screen.findByRole('menuitem', { name: nombre })).toBeInTheDocument();
  }
});

// Estar dentro de un menú no vuelve inofensivo a lo destructivo.
test('vaciar desde el menú sigue pidiendo confirmación', async () => {
  const confirmar = vi.spyOn(window, 'confirm').mockReturnValue(false);
  pinta(<Ticket {...props} />);
  await userEvent.click(await screen.findByRole('button', { name: 'Más opciones del pedido' }));
  await userEvent.click(await screen.findByRole('menuitem', { name: /Vaciar/ }));

  expect(confirmar).toHaveBeenCalled();
  expect(useTicketStore.getState().tabs[0].lines, 'se vació sin confirmar').toHaveLength(1);
  confirmar.mockRestore();
});

// Sin renglones no hay nada que vaciar: igual que hoy no se pinta el botón.
test('con el carrito vacío el menú no ofrece vaciar', async () => {
  useTicketStore.setState((s) => ({ tabs: s.tabs.map((t) => ({ ...t, lines: [] })) }));
  pinta(<Ticket {...props} />);
  await userEvent.click(await screen.findByRole('button', { name: 'Más opciones del pedido' }));

  expect(screen.queryByRole('menuitem', { name: /Vaciar/ })).toBeNull();
});

// El descuento es dinero: esconderlo detrás de un toque sería cobrar de menos sin decir por qué.
test('un descuento aplicado se ve con el menú cerrado', async () => {
  useTicketStore.getState().setDescuento('20');
  pinta(<Ticket {...props} />);

  expect(await screen.findByText(/de descuento/)).toBeInTheDocument();
  expect(await totalEnPantalla()).toBe('$75');
});

// El esquema de folio por omisión es `razas`, y ahí los nombres llegan a 20 caracteres
// ("Colorpoint Shorthair"). Lo que cede es el ancho del nombre, NUNCA la altura de un control.
test('con un nombre de folio largo ningún control del encabezado baja de 44 px', async () => {
  ponerFolio('Colorpoint Shorthair');
  pinta(<Ticket {...props} />);

  const tipo = await screen.findByRole('button', { name: /Mostrador/ });
  const menu = screen.getByRole('button', { name: 'Más opciones del pedido' });
  for (const control of [tipo, menu]) {
    const alto = getComputedStyle(control).minHeight;
    expect(parseInt(alto, 10), `${control.getAttribute('aria-label') ?? control.textContent} quedó en ${alto}`)
      .toBeGreaterThanOrEqual(44);
  }
});

// El aviso de "ya no están en el menú" vivía dentro de la caja de totales y mandó COBRAR a un
// scroll interno. El campo del menú no puede repetirlo: va ARRIBA, y lo que se encoge es la lista.
test('el campo que abre el menú no vive en la caja de totales', async () => {
  pinta(<Ticket {...props} />);
  await userEvent.click(await screen.findByRole('button', { name: 'Más opciones del pedido' }));
  await userEvent.click(await screen.findByRole('menuitem', { name: /Nombre del cliente/ }));

  const campo = await screen.findByLabelText('Nombre del cliente');
  const cobrar = screen.getByRole('button', { name: 'COBRAR' });
  const cajaDeTotales = cobrar.closest('div')?.parentElement;
  expect(cajaDeTotales?.contains(campo),
    'el campo quedó dentro de la caja de alto acotado: con él abierto, COBRAR se va a un scroll interno')
    .toBe(false);
});

// Una cuenta guardada puede traer un descuento imposible: el texto tecleado vive en el almacén y
// sobrevive a un F5. Los botones quedan apagados —eso está bien— pero el aviso que lo explica no
// puede quedarse escondido dentro de un menú.
test('un descuento imposible que viene del almacén abre su campo solo', async () => {
  useTicketStore.getState().setDescuento('1,000');
  pinta(<Ticket {...props} />);

  expect(await screen.findByText('Solo números')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'COBRAR' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Listo' }),
    'se puede cerrar el campo dejando un descuento imposible: los botones quedan apagados sin nada que lo explique')
    .toBeDisabled();
});
