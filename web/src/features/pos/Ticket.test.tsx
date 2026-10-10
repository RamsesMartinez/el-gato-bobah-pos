import { vi, describe, expect, test, beforeEach } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ComponentProps } from 'react';

import { Provider } from '../../components/ui/provider';
import type { DraftView, OrderView } from '../../types/pos';
import { armarVista, type Pendiente, type VistaCuenta } from './cuentaEnPantalla';
import { Ticket } from './Ticket';

// El ticket ya no lee un almacén: pinta la cuenta que dice el servidor (spec 030). Las cuentas de
// prueba se arman con la MISMA función que usa la pantalla, así que lo que aquí se ve es lo que
// verá quien opera.

function draft(over: Partial<DraftView> = {}): DraftView {
  return {
    id: 'd-1', orderId: null, folioName: 'Levkoy', status: 'capturando', headerVersion: 1, version: 1,
    updatedAt: '', createdAt: '', openedBy: 'Ana', serviceType: 'mostrador', customerName: null,
    platformId: null, platformOrderRef: null, deliveryFee: '0.00', discount: null,
    lines: [
      { id: 'l-1', version: 1, productId: 1, productName: 'Coca Cola 355ml', qty: '1', unitPrice: '29.00', modifiers: [], notes: '', lineTotal: '29.00', available: true },
      { id: 'l-2', version: 1, productId: 2, productName: 'Chai Miel', qty: '1', unitPrice: '45.00', modifiers: [], notes: '', lineTotal: '45.00', available: true },
    ],
    subtotal: '74.00', discountTotal: '0.00', total: '74.00', unavailable: [], ...over,
  };
}

function linea(id: number, nombre: string, total: string, over: Record<string, unknown> = {}) {
  return { id, productName: nombre, quantity: '1', unitPrice: total, lineTotal: total, delivered: '0', cancelled: false, modifiers: [], ...over };
}

function pedido(over: Partial<OrderView> = {}): OrderView {
  return {
    id: 1, number: 1, folioName: 'Khao Manee', status: 'abierta', serviceType: 'mostrador',
    deliveryPlatformId: null, platformOrderRef: null, customerName: null, subtotal: '165.00',
    discount: '0.00', deliveryFee: '0.00', total: '165.00', currency: 'MXN', paid: false,
    outstanding: '165.00', openedAt: '',
    lines: [
      linea(10, 'Kit Kat', '85.00'),
      linea(11, 'Chai Miel', '45.00'),
      linea(12, 'Chocolate Licuados', '35.00', { delivered: '1' }),
    ],
    payments: [], ...over,
  };
}

function vista(args: { draft?: DraftView; order?: OrderView; pendientes?: Pendiente[]; guardando?: boolean } = {}): VistaCuenta {
  return armarVista({
    pendientes: [], guardando: false,
    cabeceraNueva: { serviceType: 'mostrador', platformId: null, platformOrderRef: '', customerName: '' },
    ...args,
  });
}

type Props = ComponentProps<typeof Ticket>;
let handlers: Omit<Props, 'vista'>;

beforeEach(() => {
  handlers = {
    sinConexion: false, envioPorDefecto: 20, puedeCancelar: true,
    onMas: vi.fn(), onMenos: vi.fn(), onQuitar: vi.fn(), onEditLine: vi.fn(), onCabecera: vi.fn(),
    onEnviar: vi.fn(), onCobrar: vi.fn(), onDescartar: vi.fn(), onCancelarPedido: vi.fn(),
    onQuitarDeCocina: vi.fn(), onQuitarNoDisponibles: vi.fn(), onHide: vi.fn(), onImprimir: vi.fn(),
  };
});

function pinta(v: VistaCuenta, over: Partial<Props> = {}) {
  return render(<Provider><Ticket vista={v} {...handlers} {...over} /></Provider>);
}

async function abrirMenu() {
  await userEvent.click(await screen.findByRole('button', { name: 'Más opciones de la cuenta' }));
}

const alto = (el: Element) => parseInt(getComputedStyle(el).minHeight || '0', 10);

describe('las secciones del ticket (US3)', () => {
  test('una cuenta en captura solo tiene lo nuevo, con −/+', () => {
    pinta(vista({ draft: draft() }));
    expect(screen.queryByRole('heading', { name: /En cocina/ })).toBeNull();
    expect(screen.queryByRole('heading', { name: /Pagado/ })).toBeNull();
    expect(screen.getAllByRole('button', { name: 'Uno menos' })).toHaveLength(2);
    expect(screen.getAllByRole('button', { name: 'Uno más' })).toHaveLength(2);
  });

  test('un pedido con algo nuevo: «Nuevo» primero, luego «En cocina»; lo de cocina sin −/+', () => {
    pinta(vista({ order: pedido(), draft: draft({ orderId: 1, folioName: null, lines: [draft().lines![0]] }) }));
    const titulos = screen.getAllByRole('heading', { level: 3 }).map((h) => h.textContent);
    expect(titulos[0]).toMatch(/Nuevo · aún no va a cocina/);
    expect(titulos[1]).toMatch(/En cocina/);
    const cocina = screen.getByRole('region', { name: /En cocina/ });
    expect(within(cocina).queryByRole('button', { name: 'Uno menos' })).toBeNull();
    expect(within(cocina).getByText('entregado')).toBeInTheDocument();
  });

  test('lo pagado lleva candado y no tiene ⋮', () => {
    const o = pedido({
      outstanding: '80.00',
      payments: [{ id: 1, number: 1, voided: false, methodId: 1, methodName: 'Efectivo', amount: '85.00', tip: '0', reference: '', paidAt: '', receivedBy: '', split: null, lines: [{ lineId: 10, qty: '1', amount: '85.00' }] }],
    });
    pinta(vista({ order: o }));
    const pagado = screen.getByRole('region', { name: /Pagado/ });
    expect(within(pagado).getByText('Kit Kat')).toBeInTheDocument();
    expect(within(pagado).getByLabelText('Pagado')).toBeInTheDocument();
    expect(within(pagado).queryByRole('button')).toBeNull();
  });

  // Caso 8: quitar lo que ya está en cocina sale del ⋮ del renglón, no de un bote junto a la marca
  // de entregado.
  test('el ⋮ de un renglón en cocina pide quitarlo', async () => {
    pinta(vista({ order: pedido() }));
    const cocina = screen.getByRole('region', { name: /En cocina/ });
    await userEvent.click(within(cocina).getByRole('button', { name: 'Opciones de Kit Kat' }));
    await userEvent.click(await screen.findByRole('menuitem', { name: /Quitar/ }));
    expect(handlers.onQuitarDeCocina).toHaveBeenCalledWith(expect.objectContaining({ id: 10, name: 'Kit Kat' }));
  });

  // En 600 px de alto, cinco renglones de cocina empujaban lo nuevo fuera de la vista.
  test('«En cocina» con más de 3 renglones y algo nuevo se pliega solo y se despliega al tocarlo', async () => {
    const o = pedido({ lines: [1, 2, 3, 4, 5].map((i) => linea(i, `Producto ${i}`, '10.00')) });
    pinta(vista({ order: o, draft: draft({ orderId: 1 }) }));
    const boton = screen.getByRole('button', { name: /En cocina · 5/ });
    expect(screen.queryByText('Producto 1')).toBeNull();
    await userEvent.click(boton);
    expect(screen.getByText('Producto 1')).toBeInTheDocument();
  });

  test('con 3 o menos no se pliega', () => {
    pinta(vista({ order: pedido() }));
    expect(screen.getByText('Kit Kat')).toBeInTheDocument();
  });
});

describe('el pie: totales y dos botones (caso 30)', () => {
  test('una cuenta en captura: «Enviar 2 a cocina» y «Enviar y cobrar»', () => {
    pinta(vista({ draft: draft() }));
    expect(screen.getByRole('button', { name: 'Enviar 2 a cocina' })).toBeEnabled();
    expect(screen.getByRole('button', { name: /Enviar y cobrar \$74/ })).toBeEnabled();
  });

  test('un pedido sin nada nuevo: «Cobrar» lo que falta y enviar apagado', () => {
    pinta(vista({ order: pedido() }));
    expect(screen.getByRole('button', { name: /^Cobrar \$165/ })).toBeEnabled();
    expect(screen.getByRole('button', { name: /a cocina/ })).toBeDisabled();
  });

  test('un pedido con pago parcial dice lo pagado y lo que falta', () => {
    const o = pedido({ outstanding: '80.00' });
    pinta(vista({ order: o, draft: draft({ orderId: 1, folioName: null, lines: [draft().lines![0]], total: '29.00', subtotal: '29.00' }) }));
    expect(screen.getByText(/Ya pagado/)).toBeInTheDocument();
    expect(screen.getByText('$85 de $165')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Enviar y cobrar \$109/ })).toBeInTheDocument();
  });

  // Punto 11 de las decisiones del 2026-10-09: en mostrador «Cobrar» es la acción principal al
  // enviar a cocina: va primero y es el botón grande. Ya estaba así; esto impide que se pierda.
  test('cobrar va primero y es el botón principal', () => {
    pinta(vista({ draft: draft() }));
    const [primero, segundo] = within(screen.getByRole('group', { name: 'Enviar y cobrar' })).getAllByRole('button');
    expect(primero).toHaveTextContent(/cobrar/i);
    expect(segundo).toHaveTextContent(/a cocina/);
  });

  test('el pie no tiene otros controles que los dos botones', () => {
    pinta(vista({ draft: draft() }));
    const pie = screen.getByRole('group', { name: 'Enviar y cobrar' });
    expect(within(pie).getAllByRole('button')).toHaveLength(2);
  });

  test('con algo guardando los dos botones se apagan y se dice por qué', () => {
    const p: Pendiente = { opId: 'op', cuenta: 'd-1', productId: 3, name: 'Kit Kat', qty: 1, unitPrice: 85, modifiers: [], notes: '' };
    pinta(vista({ draft: draft(), pendientes: [p] }));
    expect(screen.getByRole('button', { name: /a cocina/ })).toBeDisabled();
    expect(screen.getByRole('button', { name: /cobrar/i })).toBeDisabled();
    expect(screen.getAllByText(/Guardando/).length).toBeGreaterThan(0);
  });

  test('sin conexión los dos botones se apagan y se dice por qué', () => {
    pinta(vista({ draft: draft() }), { sinConexion: true });
    expect(screen.getByRole('button', { name: /a cocina/ })).toBeDisabled();
    expect(screen.getByRole('button', { name: /cobrar/i })).toBeDisabled();
    expect(screen.getByText(/Sin conexión/)).toBeInTheDocument();
  });

  test('el motivo de un envío que falló se queda en el pie', () => {
    pinta(vista({ draft: draft() }), { motivo: 'No hay caja abierta' });
    expect(screen.getByText('No hay caja abierta')).toBeInTheDocument();
  });

  test('una cuenta vacía no ofrece enviar ni cobrar', () => {
    pinta(vista());
    expect(screen.getByText('Toca un producto para agregarlo')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /a cocina/ })).toBeDisabled();
    expect(screen.getByRole('button', { name: /cobrar/i })).toBeDisabled();
  });

  // `overflowY` sin un alto no hace scroll: la caja crece y con el teclado abierto Cobrar se va
  // abajo de la pantalla.
  test('la zona de totales tiene alto acotado', () => {
    pinta(vista({ draft: draft() }));
    const zona = screen.getByTestId('totales');
    expect(getComputedStyle(zona).maxHeight).not.toBe('none');
    expect(getComputedStyle(zona).overflowY).toBe('auto');
  });
});

describe('los renglones nuevos', () => {
  test('−, + y quitar llaman con el renglón; la papelera no está pegada a la cantidad', () => {
    pinta(vista({ draft: draft() }));
    const menos = screen.getAllByRole('button', { name: 'Uno menos' })[0];
    const quitar = screen.getAllByRole('button', { name: 'Quitar' })[0];
    expect(menos.parentElement).not.toBe(quitar.parentElement);
    fireEvent.click(menos);
    fireEvent.click(screen.getAllByRole('button', { name: 'Uno más' })[0]);
    fireEvent.click(quitar);
    expect(handlers.onMenos).toHaveBeenCalledWith(expect.objectContaining({ id: 'l-1' }));
    expect(handlers.onMas).toHaveBeenCalledWith(expect.objectContaining({ id: 'l-1' }));
    expect(handlers.onQuitar).toHaveBeenCalledWith(expect.objectContaining({ id: 'l-1' }));
  });

  test('un renglón guardando se ve así y no tiene −/+', () => {
    const p: Pendiente = { opId: 'op', cuenta: 'd-1', productId: 3, name: 'Kit Kat', qty: 1, unitPrice: 85, modifiers: [], notes: '' };
    pinta(vista({ draft: draft({ lines: [] }), pendientes: [p] }));
    // Una vez en el renglón y otra en el pie, que explica por qué los botones están apagados.
    expect(screen.getAllByText('Guardando…')).toHaveLength(2);
    expect(screen.queryByRole('button', { name: 'Uno menos' })).toBeNull();
  });

  test('avisa de lo que ya no está en el menú y lo quita de un toque', () => {
    const l = { ...draft().lines![0], available: false };
    pinta(vista({ draft: draft({ lines: [l], unavailable: ['Coca Cola 355ml'] }) }));
    expect(screen.getByText('Ya no están en el menú')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Quitar del pedido' }));
    expect(handlers.onQuitarNoDisponibles).toHaveBeenCalled();
    expect(screen.getByRole('button', { name: /a cocina/ })).toBeDisabled();
  });

  test('controles de renglón de al menos 44 px', () => {
    pinta(vista({ draft: draft() }));
    for (const b of [...screen.getAllByRole('button', { name: 'Uno menos' }), ...screen.getAllByRole('button', { name: 'Quitar' })]) {
      expect(alto(b)).toBeGreaterThanOrEqual(44);
    }
  });
});

describe('envío y descuento', () => {
  // Un envío mal escrito no puede convertirse en envío gratis: apaga los botones y lo dice.
  test('un envío mal escrito apaga los botones y no se guarda', async () => {
    pinta(vista({ draft: draft({ serviceType: 'domicilio', deliveryFee: '20.00' }) }));
    const campo = screen.getByLabelText('Costo de envío');
    await userEvent.clear(campo);
    await userEvent.type(campo, '1,5');
    fireEvent.blur(campo);
    expect(screen.getByText('Solo números')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /cobrar/i })).toBeDisabled();
    expect(handlers.onCabecera).not.toHaveBeenCalled();
  });

  test('un envío bien escrito se guarda al salir del campo', async () => {
    pinta(vista({ draft: draft({ serviceType: 'domicilio', deliveryFee: '20.00' }) }));
    const campo = screen.getByLabelText('Costo de envío');
    await userEvent.clear(campo);
    await userEvent.type(campo, '35');
    fireEvent.blur(campo);
    expect(handlers.onCabecera).toHaveBeenCalledWith({ deliveryFee: '35.00' });
  });

  test('con plataforma no se ofrece envío ni cambiar el tipo', async () => {
    pinta(vista({ draft: draft({ serviceType: 'domicilio', platformId: 3 }) }));
    expect(screen.queryByLabelText('Costo de envío')).toBeNull();
    expect(screen.queryByRole('button', { name: /Cambiar a/ })).toBeNull();
  });


  test('el descuento no ocupa alto hasta que alguien lo abre', () => {
    pinta(vista({ draft: draft() }));
    expect(screen.queryByLabelText('Descuento')).toBeNull();
  });

  test('un descuento en pesos se guarda con «Listo»', async () => {
    pinta(vista({ draft: draft() }));
    await abrirMenu();
    await userEvent.click(await screen.findByRole('menuitem', { name: /Descuento/ }));
    await userEvent.type(screen.getByLabelText('Descuento'), '20');
    await userEvent.click(screen.getByRole('button', { name: 'Listo' }));
    expect(handlers.onCabecera).toHaveBeenCalledWith({ discount: { amount: '20.00' } });
  });

  test('un descuento mal escrito o mayor que la cuenta no se puede guardar y apaga los botones', async () => {
    pinta(vista({ draft: draft() }));
    await abrirMenu();
    await userEvent.click(await screen.findByRole('menuitem', { name: /Descuento/ }));
    await userEvent.type(screen.getByLabelText('Descuento'), '500');
    expect(screen.getByRole('button', { name: 'Listo' })).toBeDisabled();
    expect(screen.getByRole('button', { name: /cobrar/i })).toBeDisabled();
  });

  test('un descuento aplicado se ve con el menú cerrado', () => {
    pinta(vista({ draft: draft({ discount: { amount: '20.00' }, discountTotal: '20.00', total: '54.00' }) }));
    expect(screen.getByText(/\$74 − \$20 de descuento/)).toBeInTheDocument();
  });
});

describe('el ⋮ de la cuenta (US5)', () => {
  test('en captura ofrece cliente, descuento y descartar; no cancelar', async () => {
    pinta(vista({ draft: draft() }));
    await abrirMenu();
    expect(await screen.findByRole('menuitem', { name: /Nombre del cliente/ })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: /Descuento/ })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: /Descartar cuenta/ })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: /Cancelar pedido/ })).toBeNull();
  });

  test('descartar llama a onDescartar', async () => {
    pinta(vista({ draft: draft() }));
    await abrirMenu();
    await userEvent.click(await screen.findByRole('menuitem', { name: /Descartar cuenta/ }));
    expect(handlers.onDescartar).toHaveBeenCalled();
  });

  // Una cuenta enviada no se cierra: solo se cancela el pedido, con motivo y permiso.
  test('un pedido enviado solo ofrece cancelar el pedido', async () => {
    pinta(vista({ order: pedido() }));
    await abrirMenu();
    expect(await screen.findByRole('menuitem', { name: /Cancelar pedido/ })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: /Descartar/ })).toBeNull();
  });

  // «Cancelar lo que falta» (dueño, 2026-10-09): entregado y pagado a medias, con permiso.
  test('un entregado pagado a medias ofrece «Cancelar lo que falta» y lo llama', async () => {
    const onCancelarResto = vi.fn();
    pinta(vista({ order: pedido({ status: 'entregada', outstanding: '100.00' }) }), { onCancelarResto });
    await abrirMenu();
    const item = await screen.findByRole('menuitem', { name: /Cancelar lo que falta/ });
    expect(alto(item)).toBeGreaterThanOrEqual(44);
    await userEvent.click(item);
    expect(onCancelarResto).toHaveBeenCalled();
  });

  test('sin pagos no se ofrece «Cancelar lo que falta»', async () => {
    pinta(vista({ order: pedido({ status: 'entregada' }) }), { onCancelarResto: vi.fn() });
    await abrirMenu();
    await screen.findByRole('menuitem', { name: /Cancelar pedido/ });
    expect(screen.queryByRole('menuitem', { name: /Cancelar lo que falta/ })).toBeNull();
  });

  test('sin permiso de cancelar, un pedido enviado no ofrece cancelarlo', async () => {
    pinta(vista({ order: pedido() }), { puedeCancelar: false });
    await abrirMenu();
    expect(await screen.findByRole('menuitem', { name: /Imprimir cuenta/ })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: /Cancelar pedido/ })).toBeNull();
  });

  // Spec 012: la cuenta impresa sale del ⋮, también de una cuenta que no se ha mandado a cocina.
  test.each([
    ['una cuenta en captura', () => vista({ draft: draft() })],
    ['un pedido enviado', () => vista({ order: pedido() })],
  ])('%s ofrece «Imprimir cuenta»', async (_n, v) => {
    pinta(v());
    await abrirMenu();
    await userEvent.click(await screen.findByRole('menuitem', { name: /Imprimir cuenta/ }));
    expect(handlers.onImprimir).toHaveBeenCalled();
  });

  test('una cuenta vacía no ofrece imprimir', async () => {
    pinta(vista());
    await abrirMenu();
    await screen.findByRole('menuitem', { name: /Nombre del cliente/ });
    expect(screen.queryByRole('menuitem', { name: /Imprimir cuenta/ })).toBeNull();
  });

  test('con un nombre largo ningún control del encabezado baja de 44 px', () => {
    pinta(vista({ draft: draft({ folioName: 'Colorpoint Shorthair' }) }));
    expect(alto(screen.getByRole('button', { name: 'Más opciones de la cuenta' }))).toBeGreaterThanOrEqual(44);
  });

  // EL NOMBRE COMPLETO (validación como usuario nuevo): «Col…» no dice de quién es la cuenta. Va
  // en su renglón, y debajo «#N · Mostrador · hora» como en el lienzo; el tipo se cambia desde ⋮.
  test('el nombre se lee completo y debajo va «#N · tipo · hora»', () => {
    pinta(vista({ order: pedido({ folioName: 'Colorpoint Shorthair', number: 33 }) }), { hora: '18:30' });
    const nombre = screen.getByRole('heading', { name: 'Colorpoint Shorthair' });
    expect(getComputedStyle(nombre).whiteSpace).not.toBe('nowrap');
    expect(screen.getByText('#33 · Mostrador · 18:30')).toBeInTheDocument();
  });

  // Un toque, como antes (revisión de tableta): pasar a domicilio es frecuente en el mostrador. El
  // botón es compacto —el nombre necesita el ancho— y dice en su nombre lo que hace.
  test('el tipo se cambia de un toque y pasar a domicilio pone el envío del negocio', async () => {
    pinta(vista({ draft: draft() }));
    const boton = screen.getByRole('button', { name: 'Cambiar a domicilio' });
    expect(alto(boton)).toBeGreaterThanOrEqual(44);
    await userEvent.click(boton);
    expect(handlers.onCabecera).toHaveBeenCalledWith({ serviceType: 'domicilio', deliveryFee: '20.00' });
  });
});

// ---------------------------------------------------------------------------------------------
// HALLAZGOS DE LA REVISIÓN DE TABLETA (after_implement).
// ---------------------------------------------------------------------------------------------
describe('lo que la revisión de tableta encontró', () => {
  // En el panel de ~300 px, «Enviar y cobrar $1,234.50» y «Enviar 3 a cocina» lado a lado no caben:
  // un botón no parte su texto, la fila desborda y Cobrar se va a un scroll lateral.
  test('los dos botones del pie van apilados, cada uno a todo lo ancho', () => {
    pinta(vista({ draft: draft() }));
    const pie = screen.getByRole('group', { name: 'Enviar y cobrar' });
    expect(getComputedStyle(pie).flexDirection).toBe('column');
  });

  // UNA REGLA PARA PLEGAR (validación como usuario nuevo): «En cocina» se veía a veces plegado y a
  // veces abierto, y tras enviar lo nuevo quedaba plegado con el ticket en blanco. Se pliega SOLO
  // cuando hay algo nuevo que capturar y más de 3 renglones enviados; sin nada nuevo, lo enviado se
  // ve completo.
  test('sin nada nuevo, «En cocina» se ve completo aunque tenga más de 3 renglones', () => {
    const otro = pedido({ id: 2, lines: [1, 2, 3, 4, 5].map((i) => linea(i, `Producto ${i}`, '10.00')) });
    pinta(vista({ order: otro }));
    expect(screen.getByText('Producto 1')).toBeInTheDocument();
  });

  test('con algo nuevo y más de 3 enviados, «En cocina» se pliega para dejar lugar a lo nuevo', () => {
    const otro = pedido({ id: 2, lines: [1, 2, 3, 4, 5].map((i) => linea(i, `Producto ${i}`, '10.00')) });
    pinta(vista({ order: otro, draft: draft({ orderId: 2 }) }));
    expect(screen.getByRole('button', { name: /En cocina · 5/ })).toBeInTheDocument();
    expect(screen.queryByText('Producto 1')).toBeNull();
  });

  test('tras enviar lo nuevo, lo enviado se ve: el ticket no queda en blanco', () => {
    const antes = pedido({ id: 2, lines: [1, 2, 3, 4].map((i) => linea(i, `Producto ${i}`, '10.00')) });
    const r = pinta(vista({ order: antes, draft: draft({ orderId: 2 }) }));
    expect(screen.queryByText('Producto 1')).toBeNull();
    const despues = pedido({ id: 2, lines: [1, 2, 3, 4, 5, 6].map((i) => linea(i, `Producto ${i}`, '10.00')) });
    r.rerender(<Provider><Ticket vista={vista({ order: despues })} {...handlers} /></Provider>);
    expect(screen.getByText('Producto 6')).toBeInTheDocument();
  });

  // Lo encontró el e2e (F3): cuatro en cocina con algo nuevo → plegado; se cobra uno por productos y
  // pasa a «Pagado», quedan tres. La sección dejaba de ser plegable —ya no hay botón para abrirla—
  // pero seguía cerrada: el encabezado «En cocina» sin un solo renglón debajo, y tres productos que
  // se cobran sin verse.
  test('si «En cocina» deja de ser plegable estando plegado, sus renglones se ven', () => {
    const cuatro = pedido({ id: 2, lines: [1, 2, 3, 4].map((i) => linea(i, `Producto ${i}`, '10.00')) });
    const nuevo = draft({ orderId: 2 });
    const r = pinta(vista({ order: cuatro, draft: nuevo }));
    expect(screen.queryByText('Producto 1')).toBeNull();
    const unoPagado = pedido({
      id: 2, outstanding: '30.00', lines: cuatro.lines,
      payments: [{ id: 1, number: 1, voided: false, methodId: 1, methodName: 'Efectivo', amount: '10.00', tip: '0', reference: '', paidAt: '', receivedBy: '', split: null, lines: [{ lineId: 4, qty: '1', amount: '10.00' }] }],
    });
    r.rerender(<Provider><Ticket vista={vista({ order: unoPagado, draft: nuevo })} {...handlers} /></Provider>);
    const cocina = screen.getByRole('region', { name: /En cocina/ });
    expect(within(cocina).getByText('Producto 1')).toBeInTheDocument();
    expect(within(cocina).getByText('Producto 3')).toBeInTheDocument();
  });

  // «Enviar 1 a cocina» con un renglón de 2 piezas: cocina recibe dos, el botón decía uno.
  test('«Enviar N a cocina» cuenta piezas, no renglones', () => {
    const l = { ...draft().lines![0], qty: '2', lineTotal: '58.00' };
    pinta(vista({ draft: draft({ lines: [l] }) }));
    expect(screen.getByRole('button', { name: 'Enviar 2 a cocina' })).toBeInTheDocument();
  });

  // El aviso del descuento en la misma fila dejaba el campo en ~10 px: se escribía a ciegas.
  test('el aviso del descuento va en su propia línea, no junto al campo', async () => {
    pinta(vista({ draft: draft() }));
    await abrirMenu();
    await userEvent.click(await screen.findByRole('menuitem', { name: /Descuento/ }));
    const campo = screen.getByLabelText('Descuento');
    await userEvent.type(campo, '500');
    const aviso = screen.getByText(/^Máx/);
    expect(campo.closest('div')?.contains(aviso)).toBe(false);
  });

  test('con algo que ya no se vende, el pie dice por qué está apagado', () => {
    const l = { ...draft().lines![0], available: false };
    pinta(vista({ draft: draft({ lines: [l] }) }));
    expect(screen.getByRole('status')).toHaveTextContent('Quita lo que ya no se vende');
  });
});
