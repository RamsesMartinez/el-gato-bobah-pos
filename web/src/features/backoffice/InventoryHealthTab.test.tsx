import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';

import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import type { InventoryHealth, HealthQuery } from '../../api/backoffice';
import { InventoryHealthTab } from './InventoryHealthTab';

// ALMACÉN › SALUD (spec 037): lo que hace que las existencias no cuadren.

const base = (over: Partial<InventoryHealth> = {}): InventoryHealth => ({
  from: '2026-09-14', to: '2026-10-13', multipleBranches: false, salesInPeriod: 40,
  unsoldDeduction: {
    total: 2, unpairedPlatformLines: 0,
    items: [
      { kind: 'product', id: 7, name: 'Shot Expreso', group: 'Bebidas', status: 'undecided', units: [{ channel: 'Mostrador', units: '2' }, { channel: 'Uber Eats', units: '1' }], pieces: '3', linesWithoutDeduction: 3, estimatedCost: '37.50', fixAt: '/catalogo/recetas?kind=product&open=product:7' },
      { kind: 'extra', id: 9, name: 'Crema Irlandesa', group: 'Sabores', status: 'extra', units: [{ channel: 'Mostrador', units: '2' }], pieces: '2', linesWithoutDeduction: 2, estimatedCost: '8.00', fixAt: '/catalogo/recetas?kind=extra&open=option:9' },
    ],
  },
  comboChoices: { total: 1, items: [{ productId: 30, name: 'Combo Ramen', slot: 'Bebida', options: 3, fixAt: '/catalogo/recetas?kind=product&open=product:30' }] },
  negativeStock: { total: 1, collapsed: true, items: [{ ingredientId: 5, name: 'Leche', unit: 'ml', onHand: '-200', branch: null, usedBy: 2, fixAt: '/almacen?tab=insumos&open=5' }] },
  noPurchases: { total: 118, of: 130, items: [] },
  malformedRecipes: { total: 1, items: [{ ownerKind: 'product', ownerId: 12, ownerName: 'Malteada', ingredient: 'Canela', problem: 'archived_ingredient', fixAt: '/catalogo/recetas?kind=product&open=product:12' }] },
  pendingRecipes: {
    undecided: { products: 24, extras: 180, fixAt: '/catalogo/recetas?kind=product&status=pending' },
    review: { products: 42, extras: 200, fixAt: '/catalogo/recetas?kind=product&status=review' },
  },
  ...over,
});

const back = vi.hoisted(() => ({ inventoryHealth: vi.fn() }));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
const uso = vi.hoisted(() => ({ medirAccion: vi.fn() }));
vi.mock('../../api/uso', async (orig) => ({ ...(await orig<object>()), medirAccion: uso.medirAccion }));

function Destino() {
  const loc = useLocation();
  return <div data-testid="destino">{loc.pathname}{loc.search}|{(loc.state as { name?: string } | null)?.name ?? ''}</div>;
}

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}><Provider>
      <MemoryRouter initialEntries={['/almacen?tab=salud']}>
        <Routes>
          <Route path="/almacen" element={<InventoryHealthTab />} />
          <Route path="*" element={<Destino />} />
        </Routes>
      </MemoryRouter>
    </Provider></QueryClientProvider>,
  );
}

describe('Almacén › Salud', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    back.inventoryHealth.mockImplementation(() => Promise.resolve(base()));
  });

  it('muestra las cinco secciones con su número y abre la primera', async () => {
    montar();
    for (const name of ['Se vende sin restar', 'Lo guardado en negativo', 'Insumos sin compras registradas', 'Recetas con un error', 'Recetas pendientes']) {
      expect(await screen.findByRole('button', { name: new RegExp(name) })).toBeInTheDocument();
    }
    expect(screen.getByRole('button', { name: /Se vende sin restar/ })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('button', { name: /Recetas con un error/ })).toHaveAttribute('aria-expanded', 'false');
    expect(uso.medirAccion).toHaveBeenCalledWith('inventario', 'ver-salud');
  });

  it('cada renglón dice qué falta, sus piezas por canal y su costo estimado, con las palabras de quien opera', async () => {
    montar();
    const shot = await screen.findByRole('button', { name: /Shot Expreso/ });
    expect(shot).toHaveTextContent('Falta decir qué resta');
    expect(shot).toHaveTextContent('Mostrador 2');
    expect(shot).toHaveTextContent('Uber Eats 1');
    expect(shot).toHaveTextContent('$37.50');
    expect(screen.getByRole('button', { name: /Crema Irlandesa/ })).toHaveTextContent('Extra que no resta nada');
    expect(screen.getByRole('button', { name: /Combo Ramen/ })).toHaveTextContent('Siempre resta el de fábrica');
    expect(screen.getByText('Las ventas anteriores no se recalculan')).toBeInTheDocument();
    // FR-019: nunca «descontar», ni «Sin decidir», ni «Extra sin ligar».
    expect(document.body.textContent).not.toMatch(/descontar|Sin decidir|Extra sin ligar/i);
  });

  it('tocar el renglón completo lleva a donde se corrige, con el nombre para la hoja', async () => {
    montar();
    const shot = await screen.findByRole('button', { name: /Shot Expreso/ });
    expect(Number.parseInt(getComputedStyle(shot).minHeight, 10)).toBeGreaterThanOrEqual(44);
    fireEvent.click(shot);
    expect(await screen.findByTestId('destino')).toHaveTextContent('/catalogo/recetas?kind=product&open=product:7|Shot Expreso');
  });

  it('sin compras registradas, lo guardado en negativo abre con un renglón que lo explica', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Lo guardado en negativo/ }));
    expect(await screen.findByText(/lo guardado solo baja/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Leche/ })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Ver los 1/ }));
    expect(await screen.findByRole('button', { name: /Leche/ })).toHaveTextContent('-200 ml');
  });

  it('«sin compras» abre resumido y pide la lista al desplegarse', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Insumos sin compras registradas/ }));
    expect(await screen.findByText('118 de 130 insumos sin compras registradas')).toBeInTheDocument();
    back.inventoryHealth.mockImplementation((q: HealthQuery) => Promise.resolve(q.section === 'no_purchases'
      ? base({ noPurchases: { total: 118, of: 130, items: [{ ingredientId: 3, name: 'Tapioca', unit: 'g', fixAt: '/almacen?tab=insumos&open=3' }] } })
      : base()));
    fireEvent.click(screen.getByRole('button', { name: /118 de 130/ }));
    expect(await screen.findByRole('button', { name: /Tapioca/ })).toBeInTheDocument();
    expect(back.inventoryHealth).toHaveBeenCalledWith(expect.objectContaining({ section: 'no_purchases' }));
  });

  it('las recetas pendientes son dos cuentas, cada una a su filtro de Recetas', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /^Recetas pendientes/ }));
    const undecided = await screen.findByRole('button', { name: /^Falta decir qué resta/ });
    expect(undecided).toHaveTextContent('24 productos · 180 extras');
    const review = screen.getByRole('button', { name: /Las cargó el sistema y nadie las ha confirmado/ });
    expect(review).toHaveTextContent('42 productos · 200 extras');
    fireEvent.click(undecided);
    expect(await screen.findByTestId('destino')).toHaveTextContent('/catalogo/recetas?kind=product&status=pending');
  });

  it('una receta con un error dice cuál', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Recetas con un error/ }));
    expect(await screen.findByRole('button', { name: /Malteada/ })).toHaveTextContent('Insumo archivado: Canela');
  });

  it('sin ventas en el periodo lo dice, y lo demás se sigue viendo', async () => {
    back.inventoryHealth.mockImplementation(() => Promise.resolve(base({ salesInPeriod: 0, unsoldDeduction: { total: 0, items: [], unpairedPlatformLines: 0 }, comboChoices: { total: 0, items: [] } })));
    montar();
    expect(await screen.findByText('No hubo ventas en este periodo')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Lo guardado en negativo/ })).toBeInTheDocument();
    expect(screen.queryByText('El inventario está sano')).not.toBeInTheDocument();
  });

  it('una empresa sin problemas ve «El inventario está sano» y no cinco ceros', async () => {
    back.inventoryHealth.mockImplementation(() => Promise.resolve(base({
      unsoldDeduction: { total: 0, items: [], unpairedPlatformLines: 0 }, comboChoices: { total: 0, items: [] },
      negativeStock: { total: 0, collapsed: false, items: [] }, noPurchases: { total: 0, of: 10, items: [] },
      malformedRecipes: { total: 0, items: [] },
      pendingRecipes: { undecided: { products: 0, extras: 0, fixAt: '' }, review: { products: 0, extras: 0, fixAt: '' } },
    })));
    montar();
    expect(await screen.findByText('El inventario está sano')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Se vende sin restar/ })).not.toBeInTheDocument();
  });

  it('un periodo que el servidor rechaza muestra el mensaje y deja la lista como estaba', async () => {
    montar();
    await screen.findByRole('button', { name: /Shot Expreso/ });
    back.inventoryHealth.mockImplementation(() => Promise.reject(new ApiError(400, 'VALIDATION', 'el periodo termina antes de empezar', 'req-1')));
    fireEvent.click(screen.getByRole('button', { name: /Periodo/ }));
    fireEvent.click(await screen.findByRole('button', { name: 'Hoy' }));
    expect(await screen.findByText(/el periodo termina antes de empezar/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Shot Expreso/ })).toBeInTheDocument();
  });

  it('el periodo se elige en una hoja, nunca en un calendario del sistema', async () => {
    const { container } = montar();
    fireEvent.click(await screen.findByRole('button', { name: /Periodo/ }));
    for (const p of ['Hoy', 'Últimos 7 días', 'Últimos 30 días', 'Este mes', 'Elegir fechas…']) {
      expect(await screen.findByRole('button', { name: p })).toBeInTheDocument();
    }
    fireEvent.click(screen.getByRole('button', { name: 'Últimos 7 días' }));
    await waitFor(() => expect(back.inventoryHealth).toHaveBeenLastCalledWith(expect.objectContaining({ from: '2026-10-07', to: '2026-10-13' })));
    expect(container.querySelector('input[type="date"], select')).toBeNull();
  });

  it('no trae su propio Page: va dentro de Almacén, que ya lo tiene', async () => {
    const { container } = montar();
    await screen.findByRole('button', { name: /Shot Expreso/ });
    const pages = [...container.querySelectorAll('div')].filter((d) => getComputedStyle(d).maxWidth === '1150px');
    expect(pages).toHaveLength(0);
    expect(within(container).queryByText(/null|undefined/)).toBeNull();
  });
});
