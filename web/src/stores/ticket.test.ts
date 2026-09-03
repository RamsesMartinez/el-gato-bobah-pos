import { describe, it, expect } from 'vitest';
import { lineTotal, ticketTotal, ticketCount, useTicketStore } from './ticket';
import type { TicketLine } from '../types/pos';

function activeLines() {
  const s = useTicketStore.getState();
  return (s.tabs.find((t) => t.id === s.activeId) ?? s.tabs[0]).lines;
}

function line(over: Partial<TicketLine>): TicketLine {
  return { lineId: 'x', productId: 1, name: 'P', unitPrice: 0, qty: 1, modifiers: [], ...over };
}

describe('ticket money math', () => {
  it('suma modificadores por unidad × cantidad', () => {
    const l = line({
      unitPrice: 45,
      qty: 2,
      modifiers: [
        { optionId: 1, groupId: 1, name: 'Perlas', priceDelta: 20, qty: 1 },
        { optionId: 2, groupId: 1, name: 'Litchi', priceDelta: 20, qty: 1 },
      ],
    });
    expect(lineTotal(l)).toBe(170); // (45+20+20)*2
  });

  it('total y conteo del ticket', () => {
    const lines = [line({ unitPrice: 30, qty: 1 }), line({ lineId: 'y', unitPrice: 12, qty: 3 })];
    expect(ticketTotal(lines)).toBe(66); // 30 + 36
    expect(ticketCount(lines)).toBe(4);
  });

  it('redondea a centavos', () => {
    const l = line({ unitPrice: 10.1, qty: 3 });
    expect(lineTotal(l)).toBe(30.3);
  });
});

describe('addLine — merge de productos directos', () => {
  it('un producto directo tocado 2 veces suma cantidad, no duplica la línea', () => {
    const s = useTicketStore.getState();
    s.clearActive();
    s.addLine({ productId: 7, name: 'Café', unitPrice: 30, qty: 1, modifiers: [] });
    s.addLine({ productId: 7, name: 'Café', unitPrice: 30, qty: 1, modifiers: [] });
    const lines = activeLines();
    expect(lines).toHaveLength(1);
    expect(lines[0].qty).toBe(2);
  });

  it('con modificadores NO fusiona: cada configuración es su propia línea', () => {
    const s = useTicketStore.getState();
    s.clearActive();
    s.addLine({ productId: 8, name: 'Crepa', unitPrice: 50, qty: 1,
      modifiers: [{ optionId: 1, groupId: 1, name: 'Nutella', priceDelta: 10, qty: 1 }] });
    s.addLine({ productId: 8, name: 'Crepa', unitPrice: 50, qty: 1,
      modifiers: [{ optionId: 2, groupId: 1, name: 'Cajeta', priceDelta: 10, qty: 1 }] });
    expect(activeLines()).toHaveLength(2);
  });

  it('con nota NO fusiona (una nota es específica de esa línea)', () => {
    const s = useTicketStore.getState();
    s.clearActive();
    s.addLine({ productId: 9, name: 'Jugo', unitPrice: 25, qty: 1, modifiers: [], notes: 'sin hielo' });
    s.addLine({ productId: 9, name: 'Jugo', unitPrice: 25, qty: 1, modifiers: [], notes: 'sin hielo' });
    expect(activeLines()).toHaveLength(2);
  });

  it('fusiona con la línea directa correcta aunque haya otro producto en medio', () => {
    const s = useTicketStore.getState();
    s.clearActive();
    s.addLine({ productId: 1, name: 'A', unitPrice: 10, qty: 1, modifiers: [] });
    s.addLine({ productId: 2, name: 'B', unitPrice: 10, qty: 1, modifiers: [] });
    s.addLine({ productId: 1, name: 'A', unitPrice: 10, qty: 1, modifiers: [] });
    const lines = activeLines();
    expect(lines).toHaveLength(2);
    expect(lines.find((l) => l.productId === 1)?.qty).toBe(2);
  });
});

test('cada cuenta lleva su propia lista de precios y una nueva arranca en mostrador', () => {
  const st = useTicketStore.getState();
  st.descartarTodo();
  expect(useTicketStore.getState().tabs[0].platformId).toBeNull();

  useTicketStore.getState().setPlatform(5);
  expect(useTicketStore.getState().tabs[0].platformId).toBe(5);

  // Una cuenta nueva no hereda la plataforma: sería la forma de cobrar precio de Uber en
  // mostrador por inercia.
  useTicketStore.getState().newTab();
  const activa = useTicketStore.getState().tabs.find((t) => t.id === useTicketStore.getState().activeId);
  expect(activa?.platformId).toBeNull();

  // Y la anterior conserva la suya: se pueden tener las dos abiertas a la vez.
  expect(useTicketStore.getState().tabs[0].platformId).toBe(5);
});

// EL RIESGO PRINCIPAL DE LOS PRECIOS POR PLATAFORMA. El precio se congela en la línea al agregarla,
// y el servidor recalcula por lista al cobrar. Sin volver a precisar lo ya agregado, armar el
// pedido en mostrador y cambiar a Uber deja la pantalla —y el ticket impreso— mostrando un total
// distinto del que se cobra. El operador lo descubre con el cliente enfrente.
describe('cambiar de lista de precios con líneas ya agregadas', () => {
  const reprecia = (l: TicketLine) => ({
    unitPrice: l.unitPrice * 1.35,
    modifiers: l.modifiers.map((m) => ({ ...m, priceDelta: m.priceDelta * 1.35 })),
  });

  it('vuelve a precisar las líneas y sus modificadores', () => {
    useTicketStore.getState().descartarTodo();
    useTicketStore.getState().addLine({
      productId: 7, name: 'Boneless', unitPrice: 100, qty: 2,
      modifiers: [{ optionId: 3, groupId: 1, name: 'BBQ', priceDelta: 20, qty: 1 }],
    });
    expect(ticketTotal(activeLines())).toBe(240); // (100 + 20) * 2

    useTicketStore.getState().setPlatform(5, reprecia);

    const l = activeLines()[0];
    expect(l.unitPrice).toBe(135);
    expect(l.modifiers[0].priceDelta).toBe(27);
    expect(ticketTotal(activeLines())).toBe(324); // (135 + 27) * 2
  });

  it('sin función de re-precio la lista cambia y los precios se quedan', () => {
    useTicketStore.getState().descartarTodo();
    useTicketStore.getState().addLine({ productId: 7, name: 'Boneless', unitPrice: 100, qty: 1, modifiers: [] });
    useTicketStore.getState().setPlatform(5);
    expect(activeLines()[0].unitPrice).toBe(100);
  });

  // Cada cuenta tiene su lista, así que re-precisar una no puede tocar a la otra: un local con una
  // cuenta de mostrador y una de Uber abiertas al mismo tiempo es el caso normal, no el raro.
  it('solo re-precia la cuenta activa', () => {
    useTicketStore.getState().descartarTodo();
    useTicketStore.getState().addLine({ productId: 7, name: 'Mostrador', unitPrice: 100, qty: 1, modifiers: [] });
    const mostrador = useTicketStore.getState().activeId;

    useTicketStore.getState().newTab();
    useTicketStore.getState().addLine({ productId: 8, name: 'Uber', unitPrice: 200, qty: 1, modifiers: [] });
    useTicketStore.getState().setPlatform(5, reprecia);

    const s = useTicketStore.getState();
    expect(s.tabs.find((t) => t.id === mostrador)?.lines[0].unitPrice).toBe(100);
    expect(activeLines()[0].unitPrice).toBe(270);
  });
});

// Corregir un precio desde la pantalla de venta tiene el mismo problema que cambiar de lista: la
// línea ya agregada conserva el precio viejo. Y aquí es peor, porque el operador acaba de corregirlo
// a propósito y da por hecho que ya quedó.
describe('repreciarTodas', () => {
  it('vuelve a precisar TODAS las cuentas, cada una con la regla de su lista', () => {
    useTicketStore.getState().descartarTodo();
    useTicketStore.getState().setPlatform(5);
    useTicketStore.getState().addLine({ productId: 7, name: 'Uber A', unitPrice: 135, qty: 1, modifiers: [] });
    const uberA = useTicketStore.getState().activeId;

    useTicketStore.getState().newTab();
    useTicketStore.getState().setPlatform(5);
    useTicketStore.getState().addLine({ productId: 7, name: 'Uber B', unitPrice: 135, qty: 1, modifiers: [] });

    useTicketStore.getState().newTab();
    useTicketStore.getState().addLine({ productId: 7, name: 'Mostrador', unitPrice: 100, qty: 1, modifiers: [] });
    const mostrador = useTicketStore.getState().activeId;

    // El precio de Uber del producto 7 se corrigió a 149; el de mostrador no se toca nunca.
    useTicketStore.getState().repreciarTodas((tab) => (l) =>
      tab.platformId === 5 ? { unitPrice: 149, modifiers: l.modifiers } : { unitPrice: l.unitPrice, modifiers: l.modifiers },
    );

    const s = useTicketStore.getState();
    const de = (id: string) => s.tabs.find((t) => t.id === id)?.lines[0].unitPrice;
    expect(de(uberA)).toBe(149);
    expect(de(mostrador)).toBe(100);
    expect(s.tabs.filter((t) => t.platformId === 5).every((t) => t.lines[0].unitPrice === 149)).toBe(true);
  });
});

// EL ENVÍO ES DE LA CUENTA, NO DE LA PANTALLA.
//
// Vivía en un `useState` de POSPage, y de ahí salían tres defectos con la misma causa: se perdía al
// recargar mientras el carrito sobrevivía —y el pedido se cobraba con el envío por defecto sin
// avisar—, se heredaba entre pestañas, y sobrevivía al cierre de la cuenta que lo capturó.
describe('el envío pertenece a la cuenta', () => {
  beforeEach(() => {
    useTicketStore.setState(useTicketStore.getInitialState(), true);
  });

  test('cada cuenta lleva el suyo', () => {
    const s = useTicketStore.getState();
    s.setEnvio('80');
    s.newTab();
    // La cuenta nueva NO hereda los $80: capturarlos para un domicilio y encontrárselos en la
    // siguiente venta es cobrarle a alguien el envío de otro.
    expect(useTicketStore.getState().tabs.at(-1)?.envio).toBe('');

    const primera = useTicketStore.getState().tabs[0].id;
    useTicketStore.getState().switchTab(primera);
    expect(useTicketStore.getState().tabs[0].envio).toBe('80');
  });

  test('vaciar la cuenta borra el envío y la plataforma', () => {
    const s = useTicketStore.getState();
    s.setEnvio('80');
    s.setPlatform(3);
    s.clearActive();

    const t = useTicketStore.getState().tabs[0];
    expect(t.envio).toBe('');
    // La plataforma también: `clearActive` reseteaba el tipo de servicio y dejaba puesta la lista de
    // Uber, así que lo capturado después salía con precio de Uber en una cuenta que decía mostrador.
    expect(t.platformId).toBeNull();
  });

  test('cerrar la cuenta se lleva su envío', () => {
    const s = useTicketStore.getState();
    s.setEnvio('80');
    s.closeTab(useTicketStore.getState().activeId);
    expect(useTicketStore.getState().tabs[0].envio).toBe('');
  });
});
