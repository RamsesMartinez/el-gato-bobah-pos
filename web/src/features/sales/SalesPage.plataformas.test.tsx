import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';

import { Provider } from '../../components/ui/provider';
import { SalesPage } from './SalesPage';
import type { SalesPage as SalesPageData, SalesSummary } from '../../api/sales';

// En su propio archivo porque simula también las cifras de plataformas: en el archivo principal
// esa consulta no se simula, y pintarla movería lo que aquellas pruebas cuentan.
const api = vi.hoisted(() => ({ list: vi.fn(), summary: vi.fn() }));
const plataformas = vi.hoisted(() => ({ summary: vi.fn() }));
vi.mock('../../api/sales', async (orig) => ({ ...(await orig<object>()), salesApi: api }));
vi.mock('../../api/settlements', async (orig) => ({ ...(await orig<object>()), settlementsApi: plataformas }));
vi.mock('../../api/pos', () => ({ posApi: { order: vi.fn(() => Promise.resolve({ lines: [] })) } }));

const pagina: SalesPageData = { range: { from: '2026-08-30', to: '2026-08-30' }, total: 0, items: [] };
const resumen: SalesSummary = {
  range: { from: '2026-08-30', to: '2026-08-30' },
  count: 0, total: '0', average: '0', tips: '0', deliveryFees: '0',
  cancelled: { count: 0, amount: '0' }, refunded: { count: 0, amount: '0' }, cancelledLines: { count: 0, amount: '0' },
  byMethod: [],
};
const cifra = { amount: '3000', orders: 4, incluye: 'x', excluye: 'y' };

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

// EL RECUADRO DE PLATAFORMAS NO CONVIVE CON UNA TABLA FILTRADA (spec 031, D15 — ya protegido).
//
// Sus cifras son del periodo completo: con la tabla filtrada a «Mostrador», los $3,000 de Uber se
// leerían junto a un total que ya no los incluye. La auditoría lo reportó; la pantalla ya apagaba la
// consulta y esta prueba es la que faltaba para que no se pierda.
describe('Ventas · recuadro de plataformas', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue(pagina);
    api.summary.mockResolvedValue(resumen);
    plataformas.summary.mockResolvedValue({
      vendido: cifra, seQuedoLaPlataforma: cifra, llegoAlBanco: cifra,
      sinLiquidar: { orders: 0 }, sinFolio: { orders: 0 },
    });
  });

  it('con un filtro de tipo de venta el recuadro desaparece', async () => {
    montar();
    expect(await screen.findByText('Vendido por plataformas')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Todos los tipos/ }));
    fireEvent.click(await screen.findByText('Mostrador'));
    await waitFor(() => {
      const ultima = api.list.mock.calls[api.list.mock.calls.length - 1][0];
      expect(ultima).toMatchObject({ serviceType: 'mostrador' });
    });
    expect(screen.queryByText('Vendido por plataformas')).not.toBeInTheDocument();
  });
});
