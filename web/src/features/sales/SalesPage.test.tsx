import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';

import { Provider } from '../../components/ui/provider';
import { SalesPage } from './SalesPage';
import type { SalesPage as SalesPageData, SalesSummary } from '../../api/sales';

const api = vi.hoisted(() => ({ list: vi.fn(), summary: vi.fn() }));
vi.mock('../../api/sales', async (orig) => ({ ...(await orig<object>()), salesApi: api }));
vi.mock('../../api/pos', () => ({ posApi: { order: vi.fn(() => Promise.resolve({ lines: [] })) } }));

const pagina: SalesPageData = {
  range: { from: '2026-08-30', to: '2026-08-30' },
  total: 2,
  items: [
    {
      id: 1, dailyNumber: 7, folioName: 'Tigre', date: '2026-08-30', openedAt: '2026-08-30T18:27:10Z', completedAt: null,
      status: 'entregada', serviceType: 'mostrador', customer: 'Sánchez', total: '275.00',
      discount: '0', discountBy: '', deliveryFee: '0', refund: '30.67', tips: '0', platform: '', platformOrderRef: '', openedBy: 'Ana', methods: 'Efectivo',
      paid: '275.00', lastRefundAt: '2026-08-30T15:08:00Z',
    },
    {
      id: 2, dailyNumber: 8, folioName: 'Nutria', date: '2026-08-30', openedAt: '2026-08-30T19:49:05Z', completedAt: null,
      status: 'abierta', serviceType: 'domicilio', customer: '', total: '0.00',
      discount: '0', discountBy: '', deliveryFee: '0', refund: '0', tips: '0', platform: 'Uber Eats',
      platformOrderRef: '4B2E9A10-77C3-4F1E-9E62-0A5C1D3F8B44', openedBy: 'Ana', methods: '',
      paid: '0', lastRefundAt: null,
    },
  ],
};

const resumen: SalesSummary = {
  range: { from: '2026-08-30', to: '2026-08-30' },
  count: 2, total: '275.00', average: '137.50', tips: '15.00', deliveryFees: '0',
  cancelled: { count: 1, amount: '50.00' },
  refunded: { count: 0, amount: '0' },
  cancelledLines: { count: 0, amount: '0' },
  pending: { count: 0, amount: '0' },
  byMethod: [{ methodId: 1, method: 'Efectivo', payments: 1, total: '275.00', tips: '15.00', refunds: '0', tipRefunds: '0' }],
};

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <MemoryRouter><SalesPage /></MemoryRouter>
      </Provider>
    </QueryClientProvider>,
  );
}

describe('pantalla de Ventas', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue(pagina);
    api.summary.mockResolvedValue(resumen);
  });

  it('arranca en hoy, sin que el operador toque nada', async () => {
    montar();
    await waitFor(() => expect(api.list).toHaveBeenCalled());
    expect(api.list.mock.calls[0][0]).toMatchObject({ preset: 'hoy' });
  });

  // El rango a la vista es lo que evita leer una cifra sin saber de qué periodo es.
  it('muestra el rango que está mirando', async () => {
    montar();
    expect(await screen.findByText('2026-08-30')).toBeInTheDocument();
  });

  it('lista las ventas con su folio, medio de pago y total', async () => {
    montar();
    expect(await screen.findByText('#7')).toBeInTheDocument();
    expect(screen.getByText('Sánchez')).toBeInTheDocument();
    // Dos veces: el renglón de la tabla y el tile del desglose por método.
    expect(screen.getAllByText('Efectivo').length).toBeGreaterThan(0);
    // Una venta sin cobrar lo dice, en vez de dejar la celda vacía y parecer un dato perdido.
    expect(screen.getByText('Sin cobrar')).toBeInTheDocument();
    // La plataforma gana al tipo de servicio: "Uber Eats" dice más que "Domicilio".
    expect(screen.getByText('Uber Eats')).toBeInTheDocument();
  });

  // La propina se marca porque NO está dentro del total. Sin la nota, quien lee suma los dos
  // números y reporta un ingreso que el negocio no tuvo.
  it('el resumen marca que la propina no entra al total', async () => {
    montar();
    expect(await screen.findByText('Propinas')).toBeInTheDocument();
    expect(screen.getByText('no entra al total')).toBeInTheDocument();
  });

  // Lo dado por perdido («cancelar lo que falta», 2026-10-09) tiene su propio tile y dice que no
  // entra al total: sumarlo con el total reportaría un cobro que nunca ocurrió.
  it('lo perdido sale aparte y fuera del total', async () => {
    api.summary.mockResolvedValue({ ...resumen, writtenOff: { count: 1, amount: '60' } });
    montar();
    expect(await screen.findByText('Perdido')).toBeInTheDocument();
    expect(screen.getByText('no entra al total · 1 pedido')).toBeInTheDocument();
  });

  // A 1024×600 la fila de recuadros desplaza a lo ancho: lo perdido iba al final, detrás de medios,
  // ventas, devoluciones y renglones cancelados, y no se veía sin deslizar (dueño, 2026-10-09).
  // Va junto a «Por cobrar», antes de los medios: es la otra cifra que NO está en el total.
  it('lo perdido va junto a «Por cobrar», antes de los medios y de las ventas', async () => {
    api.summary.mockResolvedValue({ ...resumen, writtenOff: { count: 1, amount: '60' },
      refunded: { count: 1, amount: '30' }, cancelledLines: { count: 1, amount: '10' } });
    montar();
    const perdido = await screen.findByText('Perdido');
    const ventas = screen.getByText('sin canceladas');
    const efectivo = screen.getAllByText('Efectivo')[0];
    expect(perdido.compareDocumentPosition(ventas) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(perdido.compareDocumentPosition(efectivo) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  // Un tile en cero por cada concepto llena la pantalla de ruido justo donde se busca un descuadre.
  it('no muestra conceptos que valen cero', async () => {
    montar();
    await screen.findByText('Canceladas');
    expect(screen.queryByText('Devoluciones')).not.toBeInTheDocument();
    expect(screen.queryByText('Renglones cancelados')).not.toBeInTheDocument();
  });

  it('cambiar de periodo vuelve a la primera página', async () => {
    montar();
    await waitFor(() => expect(api.list).toHaveBeenCalled());
    fireEvent.click(screen.getByRole('button', { name: 'Mes' }));
    await waitFor(() => {
      const ultima = api.list.mock.calls[api.list.mock.calls.length - 1][0];
      expect(ultima).toMatchObject({ preset: 'mes', page: 0 });
    });
  });

  // El resumen NO se vuelve a pedir al paginar ni al reordenar: no cambia con ellos, y pedirlo otra
  // vez haría que cada tap del paginador reagregara todo el rango para tirar el resultado.
  it('paginar no vuelve a pedir el resumen', async () => {
    // 45 ventas para que exista una segunda página que pedir.
    api.list.mockResolvedValue({ ...pagina, total: 45 });
    montar();
    await waitFor(() => expect(api.summary).toHaveBeenCalledTimes(1));
    // Espera a que la página cargue: con el paginador deshabilitado el clic no hace nada y el test
    // pasaría por la razón equivocada.
    const siguiente = await screen.findByRole('button', { name: /siguiente/i });
    await waitFor(() => expect(siguiente).not.toBeDisabled());

    fireEvent.click(siguiente);
    await waitFor(() => expect(api.list.mock.calls.length).toBeGreaterThan(1));
    expect(api.summary).toHaveBeenCalledTimes(1);
  });

  it('ordenar por total pide el orden al servidor, no reordena la página', async () => {
    montar();
    await waitFor(() => expect(api.list).toHaveBeenCalled());
    fireEvent.click(screen.getByText('Total'));
    await waitFor(() => {
      const ultima = api.list.mock.calls[api.list.mock.calls.length - 1][0];
      expect(ultima).toMatchObject({ sort: 'total', dir: 'desc' });
    });
  });

  it('un periodo sin ventas lo dice', async () => {
    api.list.mockResolvedValue({ ...pagina, items: [], total: 0 });
    montar();
    expect(await screen.findByText(/sin ventas en este periodo/i)).toBeInTheDocument();
  });
});

// El folio de la plataforma en la pantalla de Ventas: buscarlo, filtrar los que faltan y verlo sin
// que rompa el ancho de la tabla.
describe('SalesPage · el folio de la plataforma', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue(pagina);
    api.summary.mockResolvedValue(resumen);
  });

  it('el folio se pinta en su propio renglón, bajo el nombre de la plataforma', async () => {
    montar();
    const folio = await screen.findByText('4B2E9A10-77C3-4F1E-9E62-0A5C1D3F8B44');
    expect(folio).toBeInTheDocument();
    // Va en un elemento aparte del nombre de la plataforma, que es lo que permite truncarlo sin
    // truncar "Uber Eats".
    expect(folio.textContent).not.toContain('Uber Eats');

    // Que de verdad quede en UNA línea con elipsis NO se comprueba aquí: las medidas de Chakra son
    // clases CSS y jsdom no las resuelve, así que un assert de `white-space` pasa en verde con el
    // truncado quitado. Eso se mide en el e2e a 1024×600 (renglón Y14 de la matriz de pantallas).
  });

  it('el buscador se llama por la plataforma, no "folio" a secas', async () => {
    montar();
    // En esta misma pantalla la columna "Folio" es el número del turno. Un buscador que dijera
    // "Folio" mandaría a teclear el dato equivocado con el documento de pago en la mano.
    expect(await screen.findByLabelText(/Buscar folio de la plataforma/i)).toBeInTheDocument();
  });

  it('buscar un folio lo manda al servidor recortado', async () => {
    montar();
    const buscador = await screen.findByLabelText(/Buscar folio de la plataforma/i);
    fireEvent.change(buscador, { target: { value: '  UBER-77  ' } });
    await waitFor(() => {
      expect(api.list).toHaveBeenCalledWith(expect.objectContaining({ folio: 'UBER-77' }));
    });
    // Y el resumen recibe el MISMO filtro: si solo lo recibiera la lista, las cifras de arriba
    // describirían otro conjunto que el de abajo.
    await waitFor(() => {
      expect(api.summary).toHaveBeenCalledWith(expect.objectContaining({ folio: 'UBER-77' }));
    });
  });

  it('el filtro de pendientes es un toggle: un tap lo enciende y otro lo apaga', async () => {
    montar();
    const toggle = await screen.findByRole('button', { name: /Pendientes de folio/i });
    expect(toggle).toHaveAttribute('aria-pressed', 'false');

    fireEvent.click(toggle);
    await waitFor(() => {
      expect(api.list).toHaveBeenCalledWith(expect.objectContaining({ folioPlataforma: 'pendiente' }));
      expect(api.summary).toHaveBeenCalledWith(expect.objectContaining({ folioPlataforma: 'pendiente' }));
    });

    fireEvent.click(toggle);
    await waitFor(() => expect(toggle).toHaveAttribute('aria-pressed', 'false'));
  });
});

// Spec 029: el Total es lo que se factura, y una devolución se ve sin abrir el pedido.
describe('SalesPage · ventas netas', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({ ...pagina, items: [
      pagina.items[0],
      { ...pagina.items[1], total: '180.00', status: 'entregada' },
    ] });
    api.summary.mockResolvedValue({
      ...resumen,
      total: '2339.33',
      pending: { count: 1, amount: '180.00' },
      byMethod: [
        { methodId: 1, method: 'Efectivo', payments: 9, total: '2261.33', tips: '0', refunds: '367.67', tipRefunds: '5.00' },
        { methodId: 2, method: 'Tarjeta débito', payments: 2, total: '78.00', tips: '0', refunds: '0', tipRefunds: '0' },
      ],
    });
  });

  it('el Total dice que es lo cobrado menos lo devuelto', async () => {
    montar();
    expect(await screen.findByText('$2,339.33')).toBeInTheDocument();
    expect(screen.getByText('cobrado − devuelto')).toBeInTheDocument();
  });

  // Lo devuelto en rojo con un «−» se leía como algo por restar, y ya estaba restado.
  it('el medio dice que lo devuelto ya está restado, y la propina devuelta aparte', async () => {
    montar();
    expect(await screen.findByText('$2,261.33')).toBeInTheDocument();
    expect(screen.getByText('ya restados $367.67 devueltos')).toBeInTheDocument();
    expect(screen.getByText('+ $5 de propina devuelta')).toBeInTheDocument();
    expect(screen.queryByText(/−\$367.67 devuelto/)).not.toBeInTheDocument();
  });

  it('por cobrar va aparte y dice que no entra al total', async () => {
    montar();
    expect(await screen.findByText('Por cobrar')).toBeInTheDocument();
    expect(screen.getByText('no entra al total · 1 pedido')).toBeInTheDocument();
  });

  it('el conteo y el promedio dicen qué son', async () => {
    montar();
    expect(await screen.findByText('Ticket promedio')).toBeInTheDocument();
    expect(screen.getByText('sin canceladas')).toBeInTheDocument();
  });

  it('el renglón dice día, hora y la devolución con su momento', async () => {
    montar();
    expect(await screen.findByText(/Devuelto \$30\.67 · 30 ago/)).toBeInTheDocument();
    expect(screen.getAllByText(/30 ago/).length).toBeGreaterThan(1);
  });

  // Visto en la captura del ambiente de pruebas (2026-10-09): un pedido con su resto dado por
  // perdido seguía diciendo «Por cobrar $6» en la lista mientras el recuadro ya no lo contaba.
  it('un pedido con lo que faltaba dado por perdido no dice «Por cobrar»', async () => {
    api.list.mockResolvedValue({ ...pagina, items: [{ ...pagina.items[0], refund: '0', lastRefundAt: null,
      total: '100.00', paid: '40.00', writtenOff: '60.00' }] });
    montar();
    expect(await screen.findByText('Tigre')).toBeInTheDocument();
    expect(screen.queryByText(/^Por cobrar \$/)).toBeNull();
  });

  it('el renglón de un pedido con saldo dice cuánto falta', async () => {
    montar();
    expect(await screen.findByText('Por cobrar $180')).toBeInTheDocument();
  });

  // El pie contaba pedidos de la lista (con canceladas) y el recuadro ventas sin canceladas: «33»
  // contra «28» sin decir por qué.
  // Con tres medios o más, «Por cobrar» quedaba pasado el borde derecho.
  it('por cobrar va pegado al Total, antes de los medios', async () => {
    montar();
    const porCobrar = await screen.findByText('Por cobrar');
    const efectivo = screen.getAllByText('Efectivo').find((e) => e.closest('table') === null)!;
    expect(porCobrar.compareDocumentPosition(efectivo) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('el pie dice que cuenta pedidos de la lista', async () => {
    montar();
    expect(await screen.findByText('2 pedidos en la lista')).toBeInTheDocument();
  });
});
