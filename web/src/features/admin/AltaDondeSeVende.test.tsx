import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { NewProductDialog, NoSeVendeEnNingunLado } from './ProductsAdminPage';

// El alta de producto con «Opciones avanzadas» (spec 092): cerrado para no estorbar el alta normal,
// con el resumen a la vista, y las dos casillas encendidas por omisión.
const api = vi.hoisted(() => ({
  categories: vi.fn(() => Promise.resolve({ items: [] })),
  createProduct: vi.fn((_b: unknown) => Promise.resolve({ id: 1 })),
}));
vi.mock('../../api/admin', () => ({ adminApi: api }));

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><NewProductDialog isOpen onClose={() => {}} categoryOptions={[{ value: '1', label: 'Aguas' }]} /></Provider></QueryClientProvider>);
}

async function llenar() {
  fireEvent.change(await screen.findByLabelText('Nombre'), { target: { value: 'Limonada' } });
  fireEvent.click(screen.getByRole('button', { name: /elegir categoría/i }));
  fireEvent.click(await screen.findByRole('button', { name: 'Aguas' }));
  fireEvent.change(screen.getByLabelText('Precio'), { target: { value: '25' } });
}

describe('dónde se vende al dar de alta', () => {
  beforeEach(() => vi.clearAllMocks());

  it('opciones avanzadas cerrado, con el resumen a la vista, y el alta normal no manda sales', async () => {
    montar();
    expect(screen.getByRole('button', { name: /opciones avanzadas.*se vende en: mostrador · web/i })).toBeInTheDocument();
    expect(screen.queryByRole('checkbox', { name: /mostrador/i })).not.toBeInTheDocument();
    await llenar();
    fireEvent.click(screen.getByRole('button', { name: /^crear$/i }));
    await waitFor(() => expect(api.createProduct).toHaveBeenCalled());
    expect(api.createProduct.mock.calls[0][0]).not.toHaveProperty('sales');
  });

  it('apagar Web en el alta lo manda', async () => {
    const u = userEvent.setup();
    montar();
    await llenar();
    await u.click(screen.getByRole('button', { name: /opciones avanzadas/i }));
    await u.click(screen.getByRole('checkbox', { name: /^web/i }));
    expect(screen.getByRole('button', { name: /se vende en: mostrador$/i })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /^crear$/i }));
    await waitFor(() => expect(api.createProduct).toHaveBeenCalled());
    expect(api.createProduct.mock.calls[0][0]).toMatchObject({ sales: { counter: true, web: false, webPrice: null, allowOversell: true, description: '' } });
  });
});

describe('la lista distingue lo que no se vende en ningún lado', () => {
  it('con las dos apagadas lo dice', () => {
    render(<Provider><NoSeVendeEnNingunLado p={{ soldAtCounter: false, soldOnWeb: false }} /></Provider>);
    expect(screen.getByText('No se vende en ningún lado')).toBeInTheDocument();
  });
  it('con una encendida, o sin dato de una API anterior, no dice nada', () => {
    render(<Provider><NoSeVendeEnNingunLado p={{ soldAtCounter: false, soldOnWeb: true }} /><NoSeVendeEnNingunLado p={{}} /></Provider>);
    expect(screen.queryByText('No se vende en ningún lado')).not.toBeInTheDocument();
  });
});
