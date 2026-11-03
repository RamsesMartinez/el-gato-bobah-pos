import { describe, it, expect, vi, beforeEach } from 'vitest';
import { pickDate } from '../../testing/pickDate';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';

import { Provider } from '../../components/ui/provider';
import { SalesPage } from './SalesPage';
import type { SalesPage as SalesPageData, SalesSummary } from '../../api/sales';

const api = vi.hoisted(() => ({ list: vi.fn(), summary: vi.fn() }));
vi.mock('../../api/sales', async (orig) => ({ ...(await orig<object>()), salesApi: api }));
vi.mock('../../api/pos', () => ({
  posApi: {
    order: vi.fn(() => Promise.resolve({ lines: [] })),
    businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' })),
  },
}));

const samplePage: SalesPageData = {
  range: { from: '2026-08-30', to: '2026-08-30' },
  total: 0,
  items: [],
};

const resumen: SalesSummary = {
  range: { from: '2026-08-30', to: '2026-08-30' },
  count: 0, total: '0', average: '0', tips: '0', deliveryFees: '0',
  cancelled: { count: 0, amount: '0' },
  refunded: { count: 0, amount: '0' },
  cancelledLines: { count: 0, amount: '0' },
  pending: { count: 0, amount: '0' },
  byMethod: [],
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

// Espera a que la cola de peticiones se vacíe. Sin esto, un `expect(...).not.toHaveBeenCalled()`
// pasa por llegar ANTES que la llamada, no por haberla impedido: el test se ve verde con el defecto
// puesto, que es la peor clase de test.
const seAsienta = () => waitFor(() => expect(api.list).toHaveBeenCalled());

describe('el rango libre de fechas en Ventas', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue(samplePage);
    api.summary.mockResolvedValue(resumen);
  });

  // UN RANGO A MEDIAS NO SE MANDA.
  //
  // Con una sola fecha el servidor rechaza la petición, y aun si no lo hiciera la pantalla estaría
  // enseñando el resultado de un periodo distinto del que se está capturando. Mientras falte la
  // segunda fecha se conserva lo anterior y se dice qué falta.
  it('con una sola fecha no consulta y dice qué falta', async () => {
    const u = userEvent.setup();
    montar();
    await seAsienta();
    api.list.mockClear();

    await u.click(screen.getByRole('button', { name: 'Rango' }));
    await pickDate('Desde', '2026-08-01');

    expect(await screen.findByText(/Elige las dos fechas/)).toBeInTheDocument();
    await waitFor(() => expect(api.list).not.toHaveBeenCalled());
  });

  // Invertido devolvería CERO ventas sin error, y quien lo lea creería que no vendió ese mes.
  it('un rango invertido no consulta y lo dice', async () => {
    const u = userEvent.setup();
    montar();
    await seAsienta();
    api.list.mockClear();

    await u.click(screen.getByRole('button', { name: 'Rango' }));
    await pickDate('Desde', '2026-08-31');
    await pickDate('Hasta', '2026-08-01');

    expect(await screen.findByText(/La fecha de inicio va antes que la de fin/)).toBeInTheDocument();
    await waitFor(() => expect(api.list).not.toHaveBeenCalled());
  });

  it('con las dos fechas completas consulta ese periodo', async () => {
    const u = userEvent.setup();
    montar();
    await seAsienta();
    api.list.mockClear();

    await u.click(screen.getByRole('button', { name: 'Rango' }));
    await pickDate('Desde', '2026-08-01');
    await pickDate('Hasta', '2026-08-15');

    await waitFor(() => expect(api.list).toHaveBeenCalledWith(
      expect.objectContaining({ preset: 'rango', from: '2026-08-01', to: '2026-08-15' }),
    ));
  });

  // AL VOLVER A UN PRESET, LAS FECHAS NO VIAJAN.
  //
  // El servidor rechaza un `from` que el preset no va a usar, así que mandarlo deja la pantalla en
  // un error del que solo se sale recargando. Y la razón por la que el servidor lo rechaza es la
  // misma por la que aquí no se manda: "hoy, del 1 al 31 de enero" no significa nada.
  it('al volver a Hoy deja de mandar las fechas', async () => {
    const u = userEvent.setup();
    montar();
    await seAsienta();

    await u.click(screen.getByRole('button', { name: 'Rango' }));
    await pickDate('Desde', '2026-08-01');
    await pickDate('Hasta', '2026-08-15');
    await waitFor(() => expect(api.list).toHaveBeenCalledWith(
      expect.objectContaining({ from: '2026-08-01' }),
    ));
    api.list.mockClear();

    await u.click(screen.getByRole('button', { name: 'Hoy' }));

    await waitFor(() => expect(api.list).toHaveBeenCalled());
    const ultima = api.list.mock.calls.at(-1)?.[0];
    expect(ultima).toMatchObject({ preset: 'hoy' });
    expect(ultima).not.toHaveProperty('from');
    expect(ultima).not.toHaveProperty('to');
  });

  // EL PAGINADOR NO AVANZA SOBRE UN PERIODO QUE NO SE CONSULTÓ.
  //
  // `paginas` sale del periodo ANTERIOR (lo conserva `placeholderData`), así que con el rango a
  // medias los botones seguían vivos: tocar "Siguiente" movía el contador a "2 / 5" sobre filas que
  // no son del filtro que se está capturando.
  it('con el rango a medias el paginador queda apagado', async () => {
    const u = userEvent.setup();
    api.list.mockResolvedValue({ ...samplePage, total: 100 });
    montar();
    await seAsienta();

    await u.click(screen.getByRole('button', { name: 'Rango' }));
    await pickDate('Desde', '2026-08-01');

    expect(await screen.findByText(/Elige las dos fechas/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Siguiente' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Anterior' })).toBeDisabled();
  });

  // Un día que no ha pasado no tiene ventas: en el calendario los días posteriores a hoy (del
  // NEGOCIO) no se pueden tocar, así que un rango que termina en el futuro no se puede armar. La
  // validación del dominio sigue ahí por si llega por otro camino (domain/rangoDeFechas).
  it('no deja elegir un día que no ha pasado', async () => {
    const u = userEvent.setup();
    montar();
    await seAsienta();

    await u.click(screen.getByRole('button', { name: 'Rango' }));
    const today = new Date().toLocaleDateString('en-CA', { timeZone: 'America/Mexico_City' });
    const tomorrow = new Date(Date.parse(`${today}T12:00:00Z`) + 86_400_000).toISOString().slice(0, 10);
    fireEvent.click(screen.getByRole('button', { name: 'Hasta' }));
    const sheet = await screen.findByRole('dialog', { name: 'Hasta' });
    if (tomorrow.slice(0, 7) === today.slice(0, 7)) {
      expect(within(sheet).getByRole('button', { name: String(Number(tomorrow.slice(8))) })).toBeDisabled();
    }
    expect(within(sheet).getByRole('button', { name: String(Number(today.slice(8))) })).toBeEnabled();
  });
});
