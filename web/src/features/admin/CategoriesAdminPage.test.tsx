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

describe('lo que vio alguien nuevo de una fonda', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useCategoryView.setState({ view: 'mosaico' });
    api.categories.mockResolvedValueOnce({ items: [
      { id: 1, name: 'Bebidas frías', parentId: null, imageUrl: null, foodFamily: 'cold_drink', productCount: 1 },
      { id: 3, name: 'Embotelladas', parentId: 1, imageUrl: null, foodFamily: '', productCount: 47 },
    ] as never });
  });

  it('una subcategoría sin familia muestra la de su categoría, y su nombre no se corta con el del padre', async () => {
    const { container } = montar();
    expect(await screen.findByText('Embotelladas')).toBeInTheDocument();
    expect(screen.getAllByText('Bebidas frías').length).toBeGreaterThan(0);
    const hrefs = Array.from(container.ownerDocument.querySelectorAll('use')).map((u) => u.getAttribute('href'));
    expect(hrefs.filter((h) => h?.endsWith('#cold_drink'))).toHaveLength(2);
  });

  it('dice «1 producto», no «1 productos»', async () => {
    montar();
    expect(await screen.findByText('1 producto')).toBeInTheDocument();
    expect(screen.getByText('Bebidas frías · 47 productos')).toBeInTheDocument();
  });

  it('el selector de familia no ofrece dos «Platillo»', async () => {
    const u = userEvent.setup();
    montar();
    await u.click(await screen.findByText('Embotelladas'));
    await u.click(await screen.findByRole('button', { name: /^Platillo \(genérica\)/ }));
    expect(await screen.findAllByText(/^Platillo/)).toHaveLength(2); // el botón y la opción genérica
  });
});
