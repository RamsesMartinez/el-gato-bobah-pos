import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { RecipesPage } from './RecipesPage';
import type { RecipeQuery, RecipeRow } from '../../api/admin';

// CATÁLOGO › RECETAS: la lista de trabajo, con los casos de borde del prototipo.

const row = (id: number, name: string, extra: Partial<RecipeRow> = {}): RecipeRow => ({
  id, name, group: 'Bebidas', status: 'pending', mode: '', summary: '', lines: 0, soldPerMonth: '0', ...extra,
});
const totals = { product: { pending: 30, review: 2, done: 8 }, extra: { pending: 5, review: 0, done: 1 }, prep: { pending: 0, review: 1, done: 3 } };

const api = vi.hoisted(() => ({
  recipes: vi.fn(),
  confirmRecipes: vi.fn((_k: string, ids: number[]) => Promise.resolve({ confirmed: ids.length })),
  categories: vi.fn(() => Promise.resolve({ items: [{ id: 1, name: 'Bebidas', parentId: null }] })),
  groups: vi.fn(() => Promise.resolve({ items: [] })),
  composition: vi.fn(() => Promise.resolve({ status: '', items: [], components: [], editable: true, stamp: '', sameName: [] })),
  saveComposition: vi.fn(() => Promise.resolve({ status: 'confirmed', items: [], components: [], editable: true })),
  confirmComposition: vi.fn(),
  products: vi.fn(() => Promise.resolve({ items: [], total: 0, counts: { act: 0, inact: 0 } })),
}));
const back = vi.hoisted(() => ({
  ingredients: vi.fn(() => Promise.resolve({ items: [{ id: 3, name: 'Vaso 16 oz', baseUnitId: 5, baseUnitCode: 'pieza', baseUnitKind: 'pieza', recipeUses: 5 }] })),
  units: vi.fn(() => Promise.resolve({ items: [{ id: 5, code: 'pieza', name: 'Pieza', kind: 'pieza', toBase: '1' }] })),
  createIngredient: vi.fn(),
}));
vi.mock('../../api/admin', () => ({ adminApi: api }));
vi.mock('../../api/backoffice', () => ({ backofficeApi: back }));
vi.mock('../../api/pos', () => ({ posApi: { businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' })) } }));

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><RecipesPage /></Provider></QueryClientProvider>);
}

describe('Catálogo › Recetas', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.recipes.mockImplementation((q: RecipeQuery) => {
      if (q.q === 'zzz') return Promise.resolve({ items: [], total: 0, counts: { pending: 0, review: 0, done: 0 }, totals });
      if (q.status === 'review') {
        return Promise.resolve({ items: [row(11, 'Frappé Taro', { status: 'review', mode: 'items', summary: '180 ml Leche · 150 g Hielo · 25 g Taro', lines: 6 }), row(12, 'Frappé Oreo', { status: 'review', mode: 'items', summary: '180 ml Leche', lines: 1 })], total: 2, counts: { pending: 30, review: 2, done: 8 }, totals });
      }
      const all = Array.from({ length: 30 }, (_, i) => row(100 + i, `Producto ${i + 1}`, { soldPerMonth: String(300 - i) }));
      const off = q.offset ?? 0;
      return Promise.resolve({ items: all.slice(off, off + (q.limit ?? 25)), total: 30, counts: { pending: 30, review: 2, done: 8 }, totals });
    });
  });

  it('las pestañas dicen cuántas faltan y arriba va el avance', async () => {
    montar();
    expect(await screen.findByRole('tab', { name: /Productos\s*32/ })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: /Extras\s*5/ })).toBeInTheDocument();
    expect(screen.getByText('8 de 40')).toBeInTheDocument();
  });

  it('cada renglón dice qué le falta y cuánto se vende', async () => {
    montar();
    expect(await screen.findByText('Producto 1')).toBeInTheDocument();
    expect(screen.getAllByText('Falta la receta').length).toBeGreaterThan(0);
    expect(screen.getByText('300 al mes')).toBeInTheDocument();
  });

  it('carga de 25 en 25', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Ver 25 más · quedan 5' }));
    await waitFor(() => expect(api.recipes).toHaveBeenCalledWith(expect.objectContaining({ offset: 25 })));
    expect(await screen.findByText('Producto 30')).toBeInTheDocument();
  });

  it('busca por nombre o insumo, y sin resultados lo dice', async () => {
    montar();
    fireEvent.change(await screen.findByLabelText('Buscar'), { target: { value: 'zzz' } });
    await waitFor(() => expect(api.recipes).toHaveBeenCalledWith(expect.objectContaining({ q: 'zzz' })), { timeout: 2000 });
    expect(await screen.findByText('Nada coincide con «zzz»')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Quitar la búsqueda' }));
    expect(await screen.findByText('Producto 1')).toBeInTheDocument();
  });

  it('el orden alterna entre más vendidos y la A a la Z', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Más vendidos primero' }));
    await waitFor(() => expect(api.recipes).toHaveBeenCalledWith(expect.objectContaining({ sort: 'az' })));
  });

  it('por revisar: el resumen corta en tres y confirma solo las que se ven', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Por revisar · 2' }));
    expect(await screen.findByText(/25 g Taro · \+3 más/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar las 2 que se ven' }));
    await waitFor(() => expect(api.confirmRecipes).toHaveBeenCalledWith('product', [11, 12]));
  });

  it('al guardar una receta se abre la siguiente de la lista', async () => {
    montar();
    fireEvent.click(await screen.findByText('Producto 1'));
    fireEvent.click(await screen.findByRole('button', { name: '+ Vaso 16 oz' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.composition).toHaveBeenCalledWith('product', 101));
  });
});
