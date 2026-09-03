import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ReportsPage } from './ReportsPage';

const api = vi.hoisted(() => ({
  reportSales: vi.fn(), reportMargins: vi.fn(), reportTips: vi.fn(),
}));
vi.mock('../../api/backoffice', async (orig) => ({
  ...(await orig<object>()), backofficeApi: api,
}));
vi.mock('../../api/pos', () => ({
  posApi: { businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' })) },
}));

const rango = { from: '2026-08-05', to: '2026-09-03' };

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}><Provider><ReportsPage /></Provider></QueryClientProvider>,
  );
}

describe('pantalla de Reportes', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.reportSales.mockResolvedValue({ range: rango, byDay: [], byMethod: [] });
    api.reportMargins.mockResolvedValue({ range: rango, items: [] });
    api.reportTips.mockResolvedValue({ range: rango, byEmployee: [], byDay: [] });
  });

  // EL ENCABEZADO DICE EL PERIODO QUE EL SERVIDOR CONSULTÓ, NO UNA FRASE FIJA.
  //
  // Decía "Reportes (últimos 30 días)" escrito a mano, y lo seguiría diciendo con cualquier otro
  // rango elegido. Una cifra sin su periodo al lado no se puede auditar, y una con el periodo
  // equivocado al lado es peor: se audita mal.
  it('muestra el periodo que devolvió el servidor', async () => {
    montar();
    expect(await screen.findByText('2026-08-05 al 2026-09-03')).toBeInTheDocument();
    expect(screen.queryByText(/últimos 30 días/i)).toBeNull();
  });

  // LOS TRES REPORTES PIDEN EL MISMO PERIODO.
  //
  // Sus cifras se pintan una junto a otra y la de medios de pago se usa para cuadrar la de ventas.
  // Si una se quedara con su propio rango, la pantalla mezclaría dos periodos sin nada que lo
  // delate: es el defecto que ya tenía el servidor, donde "por medio de pago" no llevaba cota
  // superior y contestaba "de esa fecha a hoy".
  it('los tres reportes piden el mismo periodo', async () => {
    const u = userEvent.setup();
    montar();
    await waitFor(() => expect(api.reportSales).toHaveBeenCalled());

    await u.click(screen.getByRole('button', { name: 'Rango' }));
    await u.type(screen.getByLabelText('Desde'), '2026-08-01');
    await u.type(screen.getByLabelText('Hasta'), '2026-08-15');

    const periodo = { preset: 'rango', from: '2026-08-01', to: '2026-08-15' };
    await waitFor(() => {
      expect(api.reportSales).toHaveBeenCalledWith(expect.objectContaining(periodo));
      expect(api.reportMargins).toHaveBeenCalledWith(expect.objectContaining(periodo));
      expect(api.reportTips).toHaveBeenCalledWith(expect.objectContaining(periodo));
    });
  });

  // Un rango a medias no se consulta: la pantalla conserva el periodo anterior —que su encabezado
  // sigue nombrando— y dice qué falta.
  it('con media fecha no vuelve a consultar', async () => {
    const u = userEvent.setup();
    montar();
    await waitFor(() => expect(api.reportSales).toHaveBeenCalled());
    api.reportSales.mockClear();

    await u.click(screen.getByRole('button', { name: 'Rango' }));
    await u.type(screen.getByLabelText('Desde'), '2026-08-01');

    expect(await screen.findByText(/Elige las dos fechas/)).toBeInTheDocument();
    await waitFor(() => expect(api.reportSales).not.toHaveBeenCalled());
  });

  // Con un preset, las fechas NO viajan: el servidor las rechaza porque un `from` que el preset no
  // va a usar significa que la pantalla y la respuesta hablan de periodos distintos.
  it('el preset por default no manda fechas', async () => {
    montar();
    await waitFor(() => expect(api.reportSales).toHaveBeenCalled());
    const q = api.reportSales.mock.calls.at(-1)?.[0];
    expect(q).toMatchObject({ preset: '30d' });
    expect(q).not.toHaveProperty('from');
    expect(q).not.toHaveProperty('to');
  });
});
