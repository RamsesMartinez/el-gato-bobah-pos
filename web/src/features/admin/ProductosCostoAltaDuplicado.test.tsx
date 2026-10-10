import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { NewProductDialog, DuplicateProductDialog } from './ProductsAdminPage';
import { ProductEditDialog } from '../../shared/ProductEditDialog';
import type { AdminProduct } from '../../api/admin';

const api = vi.hoisted(() => ({
  categories: vi.fn(() => Promise.resolve({ items: [] })),
  createProduct: vi.fn((_b: unknown) => Promise.resolve({ id: 1 })),
  duplicateProduct: vi.fn((_id: number, _n: string, _c?: number) => Promise.resolve({ id: 2 })),
  updateProduct: vi.fn(() => Promise.resolve()),
  productGroups: vi.fn(() => Promise.resolve({ items: [] })),
  groups: vi.fn(() => Promise.resolve({ items: [] })),
}));
vi.mock('../../api/admin', () => ({ adminApi: api }));
vi.mock('../../shared/ProductGroupsManager', () => ({ ProductGroupsManager: () => null }));

const fuente = {
  id: 7, name: 'Té', price: '35', current_cost: '12.50', type: 'simple', is_active: true,
  is_favorite: false, category: 'Bebidas', categoryId: 1, availableFrom: null, availableUntil: null,
  groupCount: 0, overrideCount: 0, needsPrep: true, costSource: 'manual', manualCost: '12.50', hasRecipe: false,
} as AdminProduct;

function montar(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider>{ui}</Provider></QueryClientProvider>);
}

const minH = (el: HTMLElement) => getComputedStyle(el).minHeight;

describe('costo al crear un producto', () => {
  beforeEach(() => vi.clearAllMocks());

  it('manda el costo escrito', async () => {
    montar(<NewProductDialog isOpen onClose={() => {}} categoryOptions={[{ value: '1', label: 'Bebidas' }]} />);
    fireEvent.change(await screen.findByLabelText('Nombre'), { target: { value: 'Té verde' } });
    fireEvent.click(screen.getByRole('button', { name: /elegir categoría/i }));
    fireEvent.click(await screen.findByText('Bebidas'));
    await waitFor(() => expect(screen.getByRole('button', { name: /bebidas/i })).toBeInTheDocument());
    fireEvent.change(screen.getByLabelText('Precio'), { target: { value: '35' } });
    fireEvent.change(screen.getByLabelText(/^Costo/), { target: { value: '12.5' } });
    fireEvent.click(screen.getByRole('button', { name: /^crear$/i }));
    await waitFor(() => expect(api.createProduct).toHaveBeenCalled());
    expect(api.createProduct.mock.calls[0][0]).toMatchObject({ cost: 12.5 });
  });

  it('un costo negativo apaga «Crear» en vez de mandarse', async () => {
    montar(<NewProductDialog isOpen onClose={() => {}} categoryOptions={[]} />);
    fireEvent.change(await screen.findByLabelText(/^Costo/), { target: { value: '-3' } });
    expect(screen.getByText('Revisa el costo')).toBeInTheDocument();
  });

  it('nombre, precio y botones miden al menos 44 px', async () => {
    montar(<NewProductDialog isOpen onClose={() => {}} categoryOptions={[]} />);
    expect(minH(await screen.findByLabelText('Nombre'))).toBe('44px');
    expect(minH(screen.getByLabelText('Precio'))).toBe('44px');
    expect(minH(screen.getByRole('button', { name: /^crear$/i }))).toBe('44px');
    expect(minH(screen.getByRole('button', { name: /^cancelar$/i }))).toBe('44px');
  });
});

describe('costo al duplicar', () => {
  beforeEach(() => vi.clearAllMocks());

  it('vacío hereda el del original', async () => {
    montar(<DuplicateProductDialog source={fuente} onClose={() => {}} />);
    fireEvent.click(await screen.findByRole('button', { name: /^duplicar$/i }));
    await waitFor(() => expect(api.duplicateProduct).toHaveBeenCalled());
    expect(api.duplicateProduct.mock.calls[0][2]).toBeUndefined();
  });

  it('escrito se manda', async () => {
    montar(<DuplicateProductDialog source={fuente} onClose={() => {}} />);
    fireEvent.change(await screen.findByLabelText(/^Costo/), { target: { value: '14' } });
    fireEvent.click(screen.getByRole('button', { name: /^duplicar$/i }));
    await waitFor(() => expect(api.duplicateProduct).toHaveBeenCalled());
    expect(api.duplicateProduct.mock.calls[0][2]).toBe(14);
  });

  it('un combo muestra su costo de solo lectura con la razón', async () => {
    montar(<DuplicateProductDialog source={{ ...fuente, type: 'combo', current_cost: '30.00' }} onClose={() => {}} />);
    expect(await screen.findByText('Se suma de lo que incluye')).toBeInTheDocument();
    expect(screen.getByText('$30.00')).toBeInTheDocument();
    expect(screen.queryByLabelText(/^Costo/)).toBeNull();
  });

  it('nombre y botones miden al menos 44 px', async () => {
    montar(<DuplicateProductDialog source={fuente} onClose={() => {}} />);
    expect(minH(await screen.findByLabelText('Nombre del nuevo producto'))).toBe('44px');
    expect(minH(screen.getByRole('button', { name: /^duplicar$/i }))).toBe('44px');
    expect(minH(screen.getByRole('button', { name: /^cancelar$/i }))).toBe('44px');
  });
});

describe('editar: alto de controles', () => {
  it('nombre, precio y botones del pie miden al menos 44 px', async () => {
    montar(<ProductEditDialog product={fuente} isOpen onClose={() => {}} />);
    expect(minH(await screen.findByLabelText('Nombre'))).toBe('44px');
    expect(minH(screen.getByLabelText('Precio'))).toBe('44px');
    expect(minH(screen.getByRole('button', { name: /^guardar$/i }))).toBe('44px');
    expect(minH(screen.getByRole('button', { name: /^cancelar$/i }))).toBe('44px');
  });
});
