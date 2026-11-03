import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../components/ui/provider';
import { ApiError } from '../api/client';
import type { AdminProduct } from '../api/admin';
import { ProductEditDialog } from './ProductEditDialog';

// Spec 063: los otros nombres en la ficha del producto (maqueta «Editor «Otros nombres»», A). Van
// por su propia ruta antes del PATCH de siempre, y la revisión de tableta pidió que el texto escrito
// sin «Agregar» no se pierda al guardar (A1) ni que un choque cierre la ficha.
const calls: string[] = [];
const api = vi.hoisted(() => ({
  categories: vi.fn(() => Promise.resolve({ items: [] })),
  updateProduct: vi.fn(() => Promise.resolve()),
  setProductAltNames: vi.fn((_id: number, alts: string[]) => Promise.resolve({ altNames: alts })),
  similarProductAltNames: vi.fn(() => Promise.resolve([])),
  productGroups: vi.fn(() => Promise.resolve({ items: [] })),
  groups: vi.fn(() => Promise.resolve({ items: [] })),
}));
vi.mock('../api/admin', () => ({ adminApi: api }));
vi.mock('./ProductGroupsManager', () => ({ ProductGroupsManager: () => null }));
vi.mock('./CompositionSheet', () => ({ CompositionSheet: () => null }));

const ICE: AdminProduct = {
  id: 5, name: 'Ice Coffee', price: '65', current_cost: '10', type: 'simple', is_active: true, is_favorite: false,
  needsPrep: true, category: 'Bebidas', categoryId: 2, availableFrom: null, availableUntil: null, groupCount: 0,
  overrideCount: 0, costSource: 'manual', manualCost: '10', hasRecipe: false, altNames: ['Café helado'],
};

function pinta(onClose = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<Provider><QueryClientProvider client={qc}><ProductEditDialog product={ICE} isOpen onClose={onClose} /></QueryClientProvider></Provider>);
  return onClose;
}

beforeEach(() => {
  calls.length = 0;
  vi.clearAllMocks();
  api.setProductAltNames.mockImplementation((_id: number, alts: string[]) => { calls.push('alts'); return Promise.resolve({ altNames: alts }); });
  api.updateProduct.mockImplementation(() => { calls.push('patch'); return Promise.resolve(); });
});

describe('otros nombres en la ficha del producto', () => {
  it('muestra los que tiene y no manda la lista si no cambió', async () => {
    const u = userEvent.setup();
    const onClose = pinta();
    expect(await screen.findByText('Café helado')).toBeInTheDocument();
    await u.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(api.setProductAltNames).not.toHaveBeenCalled();
  });

  it('el texto escrito sin «Agregar» se agrega al guardar, y la lista va antes del PATCH', async () => {
    const u = userEvent.setup();
    const onClose = pinta();
    await u.type(await screen.findByRole('textbox', { name: /otro nombre/i }), 'Café frío');
    await u.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(api.setProductAltNames).toHaveBeenCalledWith(5, ['Café helado', 'Café frío']);
    expect(calls).toEqual(['alts', 'patch']);
  });

  it('un choque deja la ficha abierta con el aviso bajo el campo, sin PATCH', async () => {
    const u = userEvent.setup();
    api.setProductAltNames.mockRejectedValueOnce(new ApiError(409, 'ALT_NAME_TAKEN', '«café frío» ya es otro nombre de «Latte»', 'x', { name: 'café frío', existingId: 9 }));
    const onClose = pinta();
    await u.type(await screen.findByRole('textbox', { name: /otro nombre/i }), 'café frío');
    await u.click(screen.getByRole('button', { name: 'Guardar' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('ya es otro nombre de «Latte»');
    expect(onClose).not.toHaveBeenCalled();
    expect(api.updateProduct).not.toHaveBeenCalled();
  });

  // Revisión del front: corregir un dedazo («Ice Coffe» → «Ice Coffee») y dejar el viejo como otro
  // nombre. El servidor compara contra el nombre guardado: la lista que lo trae va después del PATCH.
  it('renombrar y dejar el nombre viejo como otro nombre: primero el PATCH, luego la lista', async () => {
    const u = userEvent.setup();
    const onClose = pinta();
    const nombre = await screen.findByDisplayValue('Ice Coffee');
    await u.clear(nombre);
    await u.type(nombre, 'Ice Coffee frío');
    await u.type(screen.getByRole('textbox', { name: /otro nombre/i }), 'Ice Coffee{Enter}');
    await u.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(calls).toEqual(['patch', 'alts']);
    expect(api.setProductAltNames).toHaveBeenLastCalledWith(5, ['Café helado', 'Ice Coffee']);
  });

  // Y al revés: subir un otro nombre a nombre principal lo quita de la lista antes del PATCH.
  it('subir un otro nombre a nombre principal lo quita de la lista primero', async () => {
    const u = userEvent.setup();
    const onClose = pinta();
    const nombre = await screen.findByDisplayValue('Ice Coffee');
    await u.clear(nombre);
    await u.type(nombre, 'Café helado');
    await u.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(calls).toEqual(['alts', 'patch']);
    expect(api.setProductAltNames).toHaveBeenCalledWith(5, []);
  });

  it('un choque del nombre (PATCH) no se pinta bajo «Otros nombres»', async () => {
    const u = userEvent.setup();
    api.updateProduct.mockRejectedValueOnce(new ApiError(409, 'ALT_NAME_TAKEN', '«Latte» ya es otro nombre de «Café con leche»', 'x'));
    pinta();
    await screen.findByText('Café helado');
    await u.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.updateProduct).toHaveBeenCalled());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
