import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { useSessionStore } from '../../stores/session';
import { useLocations } from './useLocations';

const back = vi.hoisted(() => ({ warehouses: vi.fn(), warehouseNames: vi.fn(), countPlaces: vi.fn(), createCountPlace: vi.fn() }));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

function Opciones() {
  const { options } = useLocations(undefined, { all: true });
  return <ul>{options.map((o) => <li key={o.value}>{o.label}{o.hint ? ` · ${o.hint}` : ''}</li>)}</ul>;
}

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Opciones /></QueryClientProvider>);
}

beforeEach(() => {
  useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'gerente', permissions: ['inventory.view', 'inventory.configure'] } as never });
  back.countPlaces.mockImplementation((w?: number) => Promise.resolve(w === 2
    ? [{ id: 9, name: 'Anaquel', position: 0, ingredients: 1 }]
    : [{ id: 1, name: 'Refri', position: 0, ingredients: 3 }]));
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

// La ficha y el filtro de Artículos solo ofrecían los lugares del de venta: un artículo con lugar en
// la Bodega se veía «Sin definir» y el filtro no lo encontraba (revisión de la 094).
test('con dos almacenes ofrece los lugares de los dos, cada uno con su almacén', async () => {
  back.warehouses.mockResolvedValue({ items: [
    { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 2 },
    { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 1 },
  ] });
  montar();
  expect(await screen.findByText('Anaquel · Bodega')).toBeInTheDocument();
  expect(screen.getByText('Refri · Local')).toBeInTheDocument();
});

test('con un almacén quedan como antes, sin el nombre del almacén', async () => {
  back.warehouses.mockResolvedValue({ items: [{ id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 2 }] });
  montar();
  expect(await screen.findByText('Refri')).toBeInTheDocument();
  expect(back.countPlaces).not.toHaveBeenCalledWith(2);
});
