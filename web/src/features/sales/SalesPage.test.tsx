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
      deliveryFee: '0', refund: '0', tips: '0', platform: '', platformOrderRef: '', openedBy: 'Ana', methods: 'Efectivo',
    },
    {
      id: 2, dailyNumber: 8, folioName: 'Nutria', date: '2026-08-30', openedAt: '2026-08-30T19:49:05Z', completedAt: null,
      status: 'abierta', serviceType: 'domicilio', customer: '', total: '0.00',
      deliveryFee: '0', refund: '0', tips: '0', platform: 'Uber Eats',
      platformOrderRef: '4B2E9A10-77C3-4F1E-9E62-0A5C1D3F8B44', openedBy: 'Ana', methods: '',
    },
  ],
};

const resumen: SalesSummary = {
  range: { from: '2026-08-30', to: '2026-08-30' },
  count: 2, total: '275.00', average: '137.50', tips: '15.00', deliveryFees: '0',
  cancelled: { count: 1, amount: '50.00' },
  refunded: { count: 0, amount: '0' },
  cancelledLines: { count: 0, amount: '0' },
  byMethod: [{ methodId: 1, method: 'Efectivo', payments: 1, total: '275.00', tips: '15.00' }],
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

  // Un tile en cero por cada concepto llena la pantalla de ruido justo donde se busca un descuadre.
  it('no muestra conceptos que valen cero', async () => {
    montar();
    await screen.findByText('Canceladas');
    expect(screen.queryByText('Reembolsadas')).not.toBeInTheDocument();
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
