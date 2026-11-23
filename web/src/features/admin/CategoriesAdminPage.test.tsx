import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { CategoriesAdminPage, CategoryViewSwitch } from './CategoriesAdminPage';
import { useCategoryView } from './categoryView';

const api = vi.hoisted(() => ({
  categories: vi.fn(() => Promise.resolve({ items: [
    { id: 1, name: 'Tés con perlas', parentId: null, imageUrl: null, foodFamily: 'tea', productCount: 18 },
    { id: 2, name: 'Guisados', parentId: null, imageUrl: null, foodFamily: '', productCount: 4 },
  ] })),
  setCategoryFamily: vi.fn(() => Promise.resolve({ id: 2, foodFamily: 'dish' })),
  setImage: vi.fn(), removeImage: vi.fn(),
}));
vi.mock('../../api/admin', async (orig) => ({ ...(await orig<object>()), adminApi: api }));

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <CategoryViewSwitch />
        <CategoriesAdminPage />
      </Provider>
    </QueryClientProvider>,
  );
}

describe('pestaña Categorías (spec 091, maqueta M3)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useCategoryView.setState({ view: 'mosaico' });
  });

  it('abre en mosaico, con el selector de tres vistas marcando la activa', async () => {
    montar();
    expect(await screen.findByText('Tés con perlas')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Ver como mosaico' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: 'Ver como tabla' })).toHaveAttribute('aria-pressed', 'false');
    expect(screen.getByText('18 productos')).toBeInTheDocument();
  });

  it('la tabla dice la familia de cada una; sin familia, la genérica', async () => {
    const u = userEvent.setup();
    montar();
    await u.click(screen.getByRole('button', { name: 'Ver como tabla' }));
    const row = (await screen.findByText('Guisados')).closest('tr')!;
    expect(within(row).getByText('Platillo (genérica)')).toBeInTheDocument();
    expect(localStorage.getItem('egb:catalogo:vista-categorias')).toBe('tabla');
  });

  it('si el aparato no deja guardar la vista, la pantalla sigue funcionando en mosaico', async () => {
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('bloqueado'); });
    const u = userEvent.setup();
    montar();
    await u.click(screen.getByRole('button', { name: 'Ver como tabla' }));
    expect(await screen.findByText('Guisados')).toBeInTheDocument();
    spy.mockRestore();
  });

  it('en lista y panel, elegir la familia se guarda al momento, sin un «Guardar» que olvidar', async () => {
    const u = userEvent.setup();
    montar();
    await u.click(screen.getByRole('button', { name: 'Ver como lista y panel' }));
    await u.click(await screen.findByText('Guisados'));
    await u.click(await screen.findByRole('button', { name: /^Platillo \(genérica\)/ }));
    await u.click(await screen.findByText('Ramen o sopa'));
    await waitFor(() => expect(api.setCategoryFamily).toHaveBeenCalledWith(2, 'soup'));
    expect(document.querySelector('select')).toBeNull();
  });

  it('en mosaico, tocar una categoría abre su foto y su familia', async () => {
    const u = userEvent.setup();
    montar();
    await u.click(await screen.findByText('Tés con perlas'));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('Familia de comida')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Agregar foto/ })).toBeInTheDocument();
  });
});
