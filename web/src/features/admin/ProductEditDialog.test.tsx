import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ProductEditDialog } from './ProductEditDialog';
import type { AdminProduct } from '../../api/admin';

const api = vi.hoisted(() => ({
  categories: vi.fn(() => Promise.resolve({
    items: [
      { id: 88, name: 'Snacks', parentId: null },
      { id: 90, name: 'Salados', parentId: 88 },
      { id: 40, name: 'Bebidas', parentId: null },
    ],
  })),
  updateProduct: vi.fn((_id: number, _b: unknown) => Promise.resolve()),
  productGroups: vi.fn(() => Promise.resolve({ items: [] })),
  groups: vi.fn(() => Promise.resolve({ items: [] })),
}));
vi.mock('../../api/admin', () => ({ adminApi: api }));
vi.mock('./ProductGroupsManager', () => ({ ProductGroupsManager: () => null }));

const producto = {
  id: 992, name: 'Papas Fritas - Corte Recto - CH (170g)', price: '40', current_cost: '23.40',
  type: 'simple', is_active: true, is_favorite: true, category: 'Snacks', categoryId: 88,
  availableFrom: null, availableUntil: null, groupCount: 1, overrideCount: 0,
} as AdminProduct;

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <ProductEditDialog product={producto} isOpen onClose={() => {}} />
      </Provider>
    </QueryClientProvider>,
  );
}

describe('cambiar la categoría de un producto', () => {
  beforeEach(() => vi.clearAllMocks());

  // Hasta ahora la categoría solo se fijaba al crear el producto: reacomodar el menú exigía entrar
  // a la base a mano.
  // El selector es un Picker táctil, no un <select> nativo: en una tablet de 7" el desplegable del
  // sistema tapa la pantalla con renglones de 20px.
  it('el selector arranca en la categoría actual del producto', async () => {
    montar();
    // Antes de que lleguen las categorías ya muestra la actual, no un botón vacío.
    expect(await screen.findByRole('button', { name: /Snacks/ })).toBeInTheDocument();
  });

  it('guardar manda la categoría elegida', async () => {
    montar();
    // Un tap abre la hoja; otro elige. Las subcategorías se leen con su padre para desambiguar.
    fireEvent.click(await screen.findByRole('button', { name: /Snacks/ }));
    fireEvent.click(await screen.findByText('Bebidas'));
    await waitFor(() => expect(screen.getByRole('button', { name: /Bebidas/ })).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: /^guardar$/i }));

    await waitFor(() => expect(api.updateProduct).toHaveBeenCalled());
    expect(api.updateProduct.mock.calls[0][1]).toMatchObject({ categoryId: 40 });
  });

  // El resto de los campos tiene que seguir viajando: si el guardado mandara solo la categoría, un
  // cambio de precio hecho en la misma pasada se perdería en silencio.
  it('la categoría no reemplaza al resto de los campos', async () => {
    montar();
    await screen.findByRole('button', { name: /Snacks/ });
    fireEvent.click(screen.getByRole('button', { name: /^guardar$/i }));
    await waitFor(() => expect(api.updateProduct).toHaveBeenCalled());
    expect(api.updateProduct.mock.calls[0][1]).toMatchObject({
      name: producto.name, price: 40, active: true, favorite: true, categoryId: 88,
    });
  });
});
