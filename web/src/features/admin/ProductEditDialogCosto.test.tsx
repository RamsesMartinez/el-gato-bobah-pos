import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ProductEditDialog } from '../../shared/ProductEditDialog';
import type { AdminProduct } from '../../api/admin';

const api = vi.hoisted(() => ({
  categories: vi.fn(() => Promise.resolve({ items: [] })),
  updateProduct: vi.fn((_id: number, _b: unknown) => Promise.resolve()),
  productGroups: vi.fn(() => Promise.resolve({ items: [] })),
  groups: vi.fn(() => Promise.resolve({ items: [] })),
}));
vi.mock('../../api/admin', () => ({ adminApi: api }));
vi.mock('./ProductGroupsManager', () => ({ ProductGroupsManager: () => null }));

const base = {
  id: 7, name: 'Frappé', price: '60', current_cost: '10.00', type: 'simple', is_active: true,
  is_favorite: false, category: 'Bebidas', categoryId: 1, availableFrom: null, availableUntil: null,
  groupCount: 0, overrideCount: 0, needsPrep: true,
} as AdminProduct;

function montar(p: Partial<AdminProduct>) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <ProductEditDialog product={{ ...base, ...p }} isOpen onClose={() => {}} />
      </Provider>
    </QueryClientProvider>,
  );
}

async function guardar() {
  fireEvent.click(screen.getByRole('button', { name: /^guardar$/i }));
  await waitFor(() => expect(api.updateProduct).toHaveBeenCalled());
  return api.updateProduct.mock.calls[0][1] as Record<string, unknown>;
}

describe('costo del producto', () => {
  beforeEach(() => vi.clearAllMocks());

  it('con receta se ve el costo calculado, sin campo para escribirlo, y guardar no lo toca', async () => {
    montar({ costSource: 'receta', hasRecipe: true, manualCost: null });
    expect(await screen.findByText('Sale de su receta')).toBeInTheDocument();
    expect(screen.getByText('$10.00')).toBeInTheDocument();
    expect(screen.queryByLabelText('Costo')).toBeNull();
    expect(await guardar()).not.toHaveProperty('cost');
  });

  it('pasar a costo a mano manda el monto con origen manual', async () => {
    montar({ costSource: 'receta', hasRecipe: true, manualCost: null });
    fireEvent.click(await screen.findByRole('button', { name: /poner costo a mano/i }));
    fireEvent.change(screen.getByLabelText('Costo'), { target: { value: '18.5' } });
    expect((await guardar()).cost).toEqual({ source: 'manual', amount: 18.5 });
  });

  it('un costo manual sin cambios no viaja', async () => {
    montar({ costSource: 'manual', hasRecipe: false, manualCost: '12.00', current_cost: '12.00' });
    expect(await screen.findByLabelText('Costo')).toHaveValue(12);
    expect(await guardar()).not.toHaveProperty('cost');
  });

  it('cambiar el costo manual lo manda', async () => {
    montar({ costSource: 'manual', hasRecipe: false, manualCost: '12.00', current_cost: '12.00' });
    fireEvent.change(await screen.findByLabelText('Costo'), { target: { value: '14' } });
    expect((await guardar()).cost).toEqual({ source: 'manual', amount: 14 });
  });

  // Un campo vacío no es costo cero: guardarlo así pondría el margen igual al precio sin que nadie
  // lo decidiera.
  it('con el costo borrado no se puede guardar', async () => {
    montar({ costSource: 'manual', hasRecipe: false, manualCost: '12.00', current_cost: '12.00' });
    fireEvent.change(await screen.findByLabelText('Costo'), { target: { value: '' } });
    expect(screen.getByRole('button', { name: /^guardar$/i })).toBeDisabled();
  });

  it('regresar a «de su receta» solo se ofrece si tiene receta', async () => {
    montar({ costSource: 'manual', hasRecipe: false, manualCost: '12.00' });
    await screen.findByLabelText('Costo');
    expect(screen.queryByRole('button', { name: /usar el de su receta/i })).toBeNull();
  });

  it('regresar a «de su receta» lo pide al servidor', async () => {
    montar({ costSource: 'manual', hasRecipe: true, manualCost: '12.00', current_cost: '12.00' });
    fireEvent.click(await screen.findByRole('button', { name: /usar el de su receta/i }));
    expect((await guardar()).cost).toEqual({ source: 'receta' });
  });

  it('un combo muestra su costo sin dejar capturarlo', async () => {
    montar({ type: 'combo', costSource: 'manual', hasRecipe: false, current_cost: '30.00' });
    expect(await screen.findByText('Se suma de lo que incluye')).toBeInTheDocument();
    expect(screen.queryByLabelText('Costo')).toBeNull();
  });
});

// Al elegir «de su receta» sobre un costo manual, current_cost aún es el manual: mostrarlo con la
// nota de receta sería un número falso.
describe('regresar a receta no muestra el costo manual como si fuera de receta', () => {
  beforeEach(() => vi.clearAllMocks());
  it('no pinta el monto viejo', async () => {
    montar({ costSource: 'manual', hasRecipe: true, manualCost: '12.00', current_cost: '12.00' });
    fireEvent.click(await screen.findByRole('button', { name: /usar el de su receta/i }));
    expect(screen.getByText('Se calculará de su receta al guardar')).toBeInTheDocument();
    expect(screen.queryByText('$12.00')).toBeNull();
  });
});
