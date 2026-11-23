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

describe('accesibilidad y errores de Categorías', () => {
  beforeEach(() => { vi.clearAllMocks(); useCategoryView.setState({ view: 'tabla' }); });

  it('en la tabla cada categoría se abre también con el teclado', async () => {
    const u = userEvent.setup();
    montar();
    const row = (await screen.findByText('Guisados')).closest('tr')!;
    expect(row).toHaveAttribute('tabindex', '0');
    row.focus();
    await u.keyboard('{Enter}');
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
  });

  it('si la familia no se guarda, regresa a la que tenía antes de ESTE cambio y lo dice sin «Error:»', async () => {
    useCategoryView.setState({ view: 'panel' });
    api.setCategoryFamily
      .mockResolvedValueOnce({ id: 1, foodFamily: 'coffee' })
      .mockRejectedValueOnce(new Error('sin red'));
    const u = userEvent.setup();
    montar();
    // En el panel la primera ya está abierta a la derecha. Té → Café se guarda; Café → Snack falla.
    await u.click(await screen.findByRole('button', { name: /^Té o bubble tea/ }));
    await u.click(await screen.findByText('Café'));
    await u.click(await screen.findByRole('button', { name: /^Café/ }));
    await u.click(await screen.findByText('Snack'));
    await waitFor(() => expect(api.setCategoryFamily).toHaveBeenCalledTimes(2));
    // Regresar a «Té» diría algo falso: el servidor tiene Café.
    await waitFor(() => expect(screen.getByRole('button', { name: /^Café/ })).toBeInTheDocument());
    expect(screen.queryByText(/^Error:/)).toBeNull();
  });

  it('el selector se anuncia como un grupo de vistas y la categoría elegida en el panel se marca', async () => {
    useCategoryView.setState({ view: 'panel' });
    montar();
    expect(screen.getByRole('group', { name: 'Vista de categorías' })).toBeInTheDocument();
    const first = (await screen.findAllByText('Tés con perlas'))[0].closest('button')!;
    expect(first).toHaveAttribute('aria-current', 'true');
  });
});
