import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ReportsPage } from './ReportsPage';

const api = vi.hoisted(() => ({
  reportSales: vi.fn(), reportMargins: vi.fn(), reportTips: vi.fn(), reportProductsSold: vi.fn(),
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
    api.reportProductsSold.mockResolvedValue({ range: rango, items: [] });
  });

  // LO VENDIDO DENTRO DE UN PAQUETE SE VE, SEPARADO DE LO SUELTO (spec 028).
  it('las unidades por producto distinguen sueltas de en paquetes', async () => {
    api.reportProductsSold.mockResolvedValue({
      range: rango, items: [{ product_name: 'Crepa de Nutella', alone: '12.00', in_packages: '5.00' }],
    });
    montar();
    const fila = (await screen.findByText('Crepa de Nutella')).closest('tr')!;
    expect(fila).toHaveTextContent('12');
    expect(fila).toHaveTextContent('5');
    expect(screen.getByText('En paquetes')).toBeInTheDocument();
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
      expect(api.reportProductsSold).toHaveBeenCalledWith(expect.objectContaining(periodo));
    });
  });

  // UN RANGO A MEDIAS CONSERVA LAS CIFRAS DEL PERIODO ANTERIOR, CON SU ENCABEZADO.
  //
  // Sin `placeholderData` los tres datos caían a `undefined`, los tiles a `?? 0` y el renglón del
  // periodo desaparecía: la pantalla mostraba `Ventas $0.00 · Pedidos 0 · Propinas $0.00` con las
  // tablas vacías y sin spinner —con la consulta deshabilitada `isLoading` es falso—. Tres ceros con
  // aspecto de cifra se leen como un día sin ventas, que es peor que un error.
  //
  // El test viejo solo miraba que no se llamara a la API y su comentario afirmaba lo que la pantalla
  // no hacía. Ahora mira los números.
  it('con media fecha conserva las cifras y el periodo anteriores', async () => {
    const u = userEvent.setup();
    api.reportSales.mockResolvedValue({
      range: rango, byMethod: [], byDay: [{ business_date: '2026-09-01', orders: 3, revenue: '750.00' }],
    });
    montar();
    expect(await screen.findByText('$750')).toBeInTheDocument();
    api.reportSales.mockClear();

    await u.click(screen.getByRole('button', { name: 'Rango' }));
    await u.type(screen.getByLabelText('Desde'), '2026-08-01');

    expect(await screen.findByText(/Elige las dos fechas/)).toBeInTheDocument();
    await waitFor(() => expect(api.reportSales).not.toHaveBeenCalled());
    // Lo que importa: NO se vació.
    expect(screen.getByText('$750')).toBeInTheDocument();
    expect(screen.getByText('2026-08-05 al 2026-09-03')).toBeInTheDocument();
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

  // Spec 029: cada cifra dice qué incluye.
  it('«Por medio de pago» dice que ya resta las devoluciones', async () => {
    api.reportSales.mockResolvedValue({ range: rango, byDay: [], byMethod: [{ method: 'Efectivo', payments: 3, total: '250', refunds: '50' }] });
    montar();
    expect(await screen.findByText(/ya restadas las devoluciones/i)).toBeInTheDocument();
  });

  it('un producto sin costo capturado lo dice y no presume margen', async () => {
    api.reportMargins.mockResolvedValue({ range: rango, items: [
      { product_name: 'Sodas explosivas', qty: '2', revenue: '160', cost: '0', margin: '0', uncosted_revenue: '160' },
    ] });
    montar();
    const fila = (await screen.findByText('Sodas explosivas')).closest('tr')!;
    expect(fila).toHaveTextContent('sin costo capturado');
  });

  it('Propinas por día dice el día como se lee, no como lo guarda el servidor', async () => {
    api.reportTips.mockResolvedValue({ range: rango, byEmployee: [], byDay: [{ business_date: '2026-10-08', tips: '15' }] });
    montar();
    expect(await screen.findByText('8 oct')).toBeInTheDocument();
    expect(screen.queryByText('2026-10-08')).toBeNull();
  });

  // «Ventas $1,166» arriba de medios que sumaban $480: el vendido y el cobrado se rotulan, y el
  // cobrado sale de los medios del servidor.
  it('arriba se distingue lo vendido de lo cobrado neto', async () => {
    api.reportSales.mockResolvedValue({ range: rango,
      byDay: [{ business_date: '2026-10-08', orders: 7, revenue: '1166' }],
      byMethod: [{ method: 'Efectivo', payments: 4, total: '345.83' }, { method: 'Tarjeta débito', payments: 2, total: '135' }] });
    montar();
    expect(await screen.findByText('Cobrado neto')).toBeInTheDocument();
    expect(await screen.findByText('$480.83')).toBeInTheDocument();
    expect(screen.getByText('Vendido')).toBeInTheDocument();
    expect(screen.getByText(/incluye lo que falta por cobrar/)).toBeInTheDocument();
  });
});

