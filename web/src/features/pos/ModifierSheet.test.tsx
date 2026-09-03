import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ModifierSheet } from './ModifierSheet';
import { useTicketStore } from '../../stores/ticket';
import { useSessionStore } from '../../stores/session';
import type { MenuProduct, TicketModifier } from '../../types/pos';

vi.mock('../../api/pos', () => ({ posApi: { menu: () => Promise.resolve({}) } }));
vi.mock('../../api/admin', () => ({ adminApi: {} }));
// El menú se deja mutable: la lista de precios de una plataforma es lo que distingue el caso de
// mostrador del de Didi, y sin poder cambiarla no se puede probar el que costaba dinero.
const menuMock = vi.hoisted(() => ({
  current: { platforms: [], platformPrices: {}, platformModPrices: {} } as Record<string, unknown>,
}));
vi.mock('../../hooks/useMenu', () => ({ useMenu: () => ({ data: menuMock.current }) }));

const salsa = (id: number, name: string, maxPerLine: number) =>
  ({ id, name, priceDelta: '0', maxPerLine, favorite: false });

// "Salsas" tal como está en producción: pide 2, y cada salsa admite hasta 2 iguales.
const producto = {
  id: 1, name: 'Boneless', price: '200', categoryId: 1, description: '', imageUrl: null, trackStock: false,
  groups: [{
    id: 7, title: 'Salsas', min: 2, max: 2,
    options: [salsa(10, 'Mango habanero', 2), salsa(11, 'Búfalo', 2), salsa(12, 'Sin salsa', 1)],
  }],
} as unknown as MenuProduct;

// "Dedos de Queso" tal como está en producción: un aderezo de cortesía que es OPCIONAL y de una
// sola, más un grupo obligatorio de una sola para contrastar los dos comportamientos.
const conCortesia = {
  id: 2, name: 'Dedos de Queso', price: '12', categoryId: 1, description: '', imageUrl: null, trackStock: false,
  groups: [
    { id: 20, title: 'Aderezo de cortesía', min: 0, max: 1,
      options: [salsa(30, 'Blue Cheese', 1), salsa(31, 'Ranch Cremoso', 1)] },
    { id: 21, title: 'Término', min: 1, max: 1,
      options: [salsa(40, 'Normal', 1), salsa(41, 'Bien dorado', 1)] },
  ],
} as unknown as MenuProduct;

function montar(onConfirm: (m: TicketModifier[], n: string, q: number) => void, p: MenuProduct = producto) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'cajero' } as never });
  useTicketStore.getState().descartarTodo();
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <ModifierSheet product={p} isOpen onClose={() => {}} onConfirm={onConfirm} />
      </Provider>
    </QueryClientProvider>,
  );
}

describe('elegir dos veces la misma salsa', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // La hoja recuerda la última combinación por producto en el navegador. Sin limpiar, cada caso
    // arrancaría con lo que confirmó el anterior y estaríamos probando el residuo.
    localStorage.clear();
  });

  // El caso reportado: el grupo pide dos salsas y el cliente las quiere las dos de mango.
  it('el "+" aparece al elegir una salsa repetible y manda cantidad 2', () => {
    const onConfirm = vi.fn();
    montar(onConfirm);

    // Antes de elegir nada no hay nada que repetir.
    expect(screen.queryByLabelText('Otra vez Mango habanero')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /^Mango habanero/ }));
    const mas = screen.getByLabelText('Otra vez Mango habanero');

    fireEvent.click(mas);
    expect(screen.getByRole('button', { name: /Mango habaneros*×2/ })).toBeInTheDocument();
    // Al llenar el grupo el "+" desaparece: ya no cabe otra.
    expect(screen.queryByLabelText('Otra vez Mango habanero')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /agregar|confirmar|listo/i }));
    expect(onConfirm).toHaveBeenCalled();
    const mods = onConfirm.mock.calls[0][0] as TicketModifier[];
    expect(mods).toHaveLength(1);
    expect(mods[0]).toMatchObject({ optionId: 10, qty: 2 });
  });

  // 818 de las opciones tienen maxPerLine 1. Para ellas nada cambia: sin "+", el chip sigue siendo
  // un toggle de un toque.
  it('una opción que no admite repetirse no ofrece el "+"', () => {
    montar(vi.fn());
    fireEvent.click(screen.getByRole('button', { name: /^Sin salsa/ }));
    expect(screen.queryByLabelText('Otra vez Sin salsa')).not.toBeInTheDocument();
  });

  // El "+" solo suma. Con el grupo lleno por dos salsas distintas no aparece, porque para crecer
  // tendría que quitarle una a la otra — y eso no puede esconderse detrás de un "+".
  it('con el grupo lleno por dos salsas distintas no ofrece repetir', () => {
    montar(vi.fn());
    fireEvent.click(screen.getByRole('button', { name: /^Mango habanero/ }));
    fireEvent.click(screen.getByRole('button', { name: /^Búfalo/ }));
    expect(screen.queryByLabelText('Otra vez Mango habanero')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Otra vez Búfalo')).not.toBeInTheDocument();
  });

  // Quitar sigue costando UN toque. Es la razón por la que el "+" existe en vez de hacer que el
  // toque repetido incremente: corregir una salsa mal elegida es más frecuente que pedirla doble.
  it('tocar el chip lo quita de un solo toque, aunque vaya en ×2', () => {
    montar(vi.fn());
    const chip = () => screen.getByRole('button', { name: /^Mango habanero/ });
    fireEvent.click(chip());
    fireEvent.click(screen.getByLabelText('Otra vez Mango habanero'));
    expect(screen.getByRole('button', { name: /Mango habaneros*×2/ })).toBeInTheDocument();

    fireEvent.click(chip());
    expect(screen.queryByRole('button', { name: /Mango habaneros*×2/ })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Otra vez Mango habanero')).not.toBeInTheDocument();
  });
});

// La memoria de la última combinación es lo que rompe el círculo: sin ventas capturadas no hay
// ranking que pre-marque, y sin pre-marcado capturar cuesta los taps que hacen que no se capture.
describe('recordar cómo se pidió la última vez', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  it('la segunda vez la hoja abre con las salsas de la vez pasada', () => {
    const primera = vi.fn();
    const { unmount } = montar(primera);
    fireEvent.click(screen.getByRole('button', { name: /^Mango habanero/ }));
    fireEvent.click(screen.getByLabelText('Otra vez Mango habanero'));
    fireEvent.click(screen.getByRole('button', { name: /agregar|confirmar|listo/i }));
    unmount();

    // Al reabrir, el grupo ya está cumplido: se puede confirmar sin tocar una salsa.
    const segunda = vi.fn();
    montar(segunda);
    fireEvent.click(screen.getByRole('button', { name: /agregar|confirmar|listo/i }));
    expect(segunda).toHaveBeenCalled();
    const mods = segunda.mock.calls[0][0] as TicketModifier[];
    expect(mods).toEqual([expect.objectContaining({ optionId: 10, qty: 2 })]);
  });

  // La hoja SIGUE abriendo y las marcas se ven: el operador puede corregirlas de un toque. Eso es
  // lo que separa esto de agregar la línea a ciegas.
  it('lo recordado se puede corregir antes de confirmar', () => {
    const primera = vi.fn();
    const { unmount } = montar(primera);
    fireEvent.click(screen.getByRole('button', { name: /^Mango habanero/ }));
    fireEvent.click(screen.getByLabelText('Otra vez Mango habanero'));
    fireEvent.click(screen.getByRole('button', { name: /agregar|confirmar|listo/i }));
    unmount();

    const segunda = vi.fn();
    montar(segunda);
    // Se ve marcado, y tocarlo lo quita.
    expect(screen.getByRole('button', { name: /Mango habanero\s*×2/ })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /^Mango habanero/ }));
    expect(screen.queryByRole('button', { name: /Mango habanero\s*×2/ })).not.toBeInTheDocument();
  });
});

// EL ADEREZO DE CORTESÍA MARCADO POR ERROR NO SE PODÍA QUITAR.
//
// El grupo dice "opcional" y aun así, una vez tocado, el toque repetido volvía a elegir lo mismo:
// la única salida era borrar el renglón de la cuenta y recapturarlo. La línea se iba a cocina con
// un aderezo que el cliente no pidió.
describe('un grupo de una sola opción', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  it('siendo opcional, tocar la elegida la quita y no manda nada', () => {
    const onConfirm = vi.fn();
    montar(onConfirm, conCortesia);
    const blue = () => screen.getByRole('button', { name: /^Blue Cheese/ });

    fireEvent.click(blue());
    fireEvent.click(blue());

    fireEvent.click(screen.getByRole('button', { name: /agregar|confirmar|listo/i }));
    const mods = onConfirm.mock.calls[0][0] as TicketModifier[];
    expect(mods.some((m) => m.optionId === 30), 'el aderezo desmarcado no puede viajar a cocina').toBe(false);
  });

  it('siendo opcional, tocar otra la reemplaza sin dejar dos', () => {
    const onConfirm = vi.fn();
    montar(onConfirm, conCortesia);
    fireEvent.click(screen.getByRole('button', { name: /^Blue Cheese/ }));
    fireEvent.click(screen.getByRole('button', { name: /^Ranch Cremoso/ }));

    fireEvent.click(screen.getByRole('button', { name: /agregar|confirmar|listo/i }));
    const mods = onConfirm.mock.calls[0][0] as TicketModifier[];
    expect(mods.filter((m) => m.groupId === 20)).toHaveLength(1);
    expect(mods.find((m) => m.groupId === 20)?.optionId).toBe(31);
  });

  // En el OBLIGATORIO el toque repetido no vacía: hay que elegir algo de todos modos, y dejarlo en
  // blanco solo apaga el botón de agregar y obliga a deshacer. Cambiar de opción sigue costando un
  // toque, que es lo que ahí se necesita.
  it('siendo obligatorio, tocar la elegida no lo deja vacío', () => {
    const onConfirm = vi.fn();
    montar(onConfirm, conCortesia);
    // La hoja pre-marca el obligatorio al abrir; se toca esa misma.
    fireEvent.click(screen.getByRole('button', { name: /^Normal/ }));

    const agregar = screen.getByRole('button', { name: /agregar|falta/i });
    expect(agregar, 'vaciar el grupo obligatorio apagaría el botón').toBeEnabled();
    fireEvent.click(agregar);
    expect(onConfirm.mock.calls[0][0] as TicketModifier[]).toEqual(
      expect.arrayContaining([expect.objectContaining({ groupId: 21 })]),
    );
  });
});

// EL BOTÓN COBRABA EL PRECIO DE MOSTRADOR CON LOS EXTRAS DE LA PLATAFORMA.
//
// Reproducido con los números reales del ambiente de pruebas: Frappé, base $65, con precio capturado
// a mano de $100 en Didi, y tres extras con su propia excepción de $20 cada uno.
//
//   encabezado ...... $100 en Didi   (precioDeLista)
//   botón ........... $125           (product.price + deltas de Didi)  ← la mezcla
//   servidor ........ $160           (precio de Didi + deltas de Didi)
//
// El mismo componente sacaba la cifra de dos fuentes: el encabezado de `precioDeLista` y el botón
// del precio base. El operador lee el botón y le dice $125 al cliente; el ticket sale en $160.
describe('el total en la lista de una plataforma', () => {
  const conDidi = {
    id: 630, name: 'Frappé', price: '65', categoryId: 1, description: '', imageUrl: null,
    trackStock: false,
    groups: [{
      id: 40, title: 'Tipo de leche', min: 1, max: 1,
      options: [
        { id: 642, name: 'Leche Deslactosada', priceDelta: '12', maxPerLine: 1, favorite: false },
        { id: 643, name: 'Leche Entera', priceDelta: '0', maxPerLine: 1, favorite: false },
      ],
    }],
  } as unknown as MenuProduct;

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    menuMock.current = {
      platforms: [{ id: 1, name: 'Didi', markupPct: '35' }],
      platformPrices: { 1: { 630: '100' } },
      platformModPrices: { 1: { 642: '20' } },
      products: [conDidi],
    };
  });
  afterEach(() => { menuMock.current = { platforms: [], platformPrices: {}, platformModPrices: {} }; });

  it('suma el precio DE LA PLATAFORMA, no el de mostrador', async () => {
    const onConfirm = vi.fn();
    montar(onConfirm, conDidi);
    useTicketStore.getState().setPlatform(1);

    // La leche deslactosada cuesta $20 en Didi (excepción), no sus $12 de mostrador.
    fireEvent.click(await screen.findByRole('button', { name: /^Leche Deslactosada/ }));

    // $100 de Didi + $20 del extra. Con el precio base serían $85, y eso es lo que se cobraba de
    // menos: $15 por pieza que el servidor sí carga.
    expect(screen.getByRole('button', { name: /Agregar 1 · \$120/ })).toBeInTheDocument();
  });

  it('el encabezado y el botón salen del mismo precio', async () => {
    montar(vi.fn(), conDidi);
    useTicketStore.getState().setPlatform(1);

    // Sin extras, el botón tiene que decir exactamente lo que el encabezado promete.
    expect(await screen.findByText(/\$100 en Didi/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Agregar 1 · \$100/ })).toBeInTheDocument();
  });
});

// CORREGIR EL PRECIO DE LA PLATAFORMA SIN SALIR DE LA HOJA.
//
// Con Didi activo, cambiar el precio del Frappé obligaba a cerrar la hoja, ir al catálogo, editarlo
// y volver a armar el pedido — con el repartidor esperando. El precio ya se muestra aquí; lo que
// faltaba era poder tocarlo.
//
// Solo con una plataforma activa: en mostrador el precio se edita en el catálogo, y confundir las
// dos listas es el error que esta pantalla no puede permitir.
describe('corregir el precio del producto desde la hoja', () => {
  const conDidi2 = {
    id: 630, name: 'Frappé', price: '65', categoryId: 1, description: '', imageUrl: null,
    trackStock: false,
    groups: [{
      id: 40, title: 'Tipo de leche', min: 1, max: 1,
      options: [{ id: 643, name: 'Leche Entera', priceDelta: '0', maxPerLine: 1, favorite: false }],
    }],
  } as unknown as MenuProduct;

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    menuMock.current = {
      platforms: [{ id: 1, name: 'Didi', markupPct: '35' }],
      platformPrices: { 1: { 630: '100' } },
      platformModPrices: {},
      products: [conDidi2],
    };
  });
  afterEach(() => { menuMock.current = { platforms: [], platformPrices: {}, platformModPrices: {} }; });

  it('el precio de la plataforma es un control, no un rótulo', async () => {
    montar(vi.fn(), conDidi2);
    useTicketStore.getState().setPlatform(1);

    const precio = await screen.findByRole('button', { name: /Corregir el precio/ });
    expect(precio).toHaveTextContent('$100');
    expect(precio).toHaveTextContent('Didi');
  });

  it('tocarlo abre el diálogo con el desglose de dónde sale el número', async () => {
    montar(vi.fn(), conDidi2);
    useTicketStore.getState().setPlatform(1);

    fireEvent.click(await screen.findByRole('button', { name: /Corregir el precio/ }));

    // El desglose no es adorno: se corrige un número que el sistema calculó, y sin ver de dónde
    // salió se corrige a ciegas. $65 de mostrador, $87.75 calculado con el 35%, $100 vigente.
    expect((await screen.findAllByText(/65/)).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/87\.75/).length).toBeGreaterThan(0);
  });

  // En mostrador NO se ofrece. El precio base se cambia en el catálogo; ofrecerlo aquí haría que
  // alguien corrigiera el de mostrador creyendo que corrige el de Didi.
  it('sin plataforma activa no se ofrece corregir nada', async () => {
    menuMock.current = { platforms: [], platformPrices: {}, platformModPrices: {} };
    montar(vi.fn(), conDidi2);

    await screen.findByText(/65 base/);
    expect(screen.queryByRole('button', { name: /Corregir el precio/ })).toBeNull();
  });

  // La barra decía en prosa cómo corregir un extra. Es una instrucción operativa, y en una hoja
  // donde el alto escasea va detrás de un icono de ayuda, no ocupando una fila.
  it('las instrucciones viven detrás del icono de ayuda, no en la barra', async () => {
    montar(vi.fn(), conDidi2);
    useTicketStore.getState().setPlatform(1);

    expect(screen.queryByText(/Mantén presionado un extra/)).toBeNull();
    fireEvent.click(await screen.findByRole('button', { name: /Cómo corregir precios/ }));
    expect(await screen.findByText(/Mantén presionado/)).toBeInTheDocument();
  });
});
