import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../components/ui/provider';
import type { AdminProduct } from '../api/admin';
import { ProductEditDialog } from './ProductEditDialog';

// La ficha del producto a pantalla completa con pestañas por tema (spec 092, maqueta «Ficha del
// producto», G2). Lo que se rompe si no: un bloque de hoy que se pierde al repartir en pestañas, lo
// capturado que se borra al cambiar de pestaña, un Guardar que no dice por qué no guarda y un precio
// web que se manda como cero.
const api = vi.hoisted(() => ({
  categories: vi.fn(() => Promise.resolve({ items: [] })),
  updateProduct: vi.fn((_id: number, _b: unknown) => Promise.resolve()),
  setProductAltNames: vi.fn((_id: number, alts: string[]) => Promise.resolve({ altNames: alts })),
  similarProductAltNames: vi.fn(() => Promise.resolve([])),
  productGroups: vi.fn(() => Promise.resolve({ items: [] })),
  groups: vi.fn(() => Promise.resolve({ items: [] })),
}));
vi.mock('../api/admin', () => ({ adminApi: api }));
vi.mock('./ProductGroupsManager', () => ({ ProductGroupsManager: () => <div>Lista de grupos</div> }));
vi.mock('./CompositionSheet', () => ({ CompositionSheet: () => null }));

const HORCHATA: AdminProduct = {
  id: 7, name: 'Agua de horchata', price: '35', current_cost: '9.5', type: 'simple', is_active: true, is_favorite: false,
  needsPrep: false, category: 'Aguas', categoryId: 3, availableFrom: null, availableUntil: null, groupCount: 1,
  overrideCount: 0, costSource: 'manual', manualCost: '9.5', hasRecipe: true, altNames: [],
  soldAtCounter: true, soldOnWeb: true, webPrice: null, allowOversell: true, measuresStock: true, description: '',
};

function pinta(p: Partial<AdminProduct> = {}, onClose = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<Provider><QueryClientProvider client={qc}><ProductEditDialog product={{ ...HORCHATA, ...p }} isOpen onClose={onClose} /></QueryClientProvider></Provider>);
  return onClose;
}

const tab = (name: RegExp) => screen.getByRole('tab', { name });
const sent = () => api.updateProduct.mock.calls[0][1] as { sales?: Record<string, unknown> };

beforeEach(() => vi.clearAllMocks());

describe('ficha del producto por pestañas', () => {
  it('tiene las cinco pestañas y abre en General', async () => {
    pinta();
    for (const n of [/^general/i, /^disponibilidad/i, /^venta en web/i, /^receta e inventario/i, /^extras/i]) {
      expect(tab(n)).toBeInTheDocument();
    }
    expect(tab(/^general/i)).toHaveAttribute('aria-selected', 'true');
    expect(await screen.findByLabelText('Nombre')).toBeInTheDocument();
  });

  it('cada bloque de hoy vive en una sola pestaña', async () => {
    const u = userEvent.setup();
    pinta();
    expect(screen.getByText('Pasa por cocina')).toBeInTheDocument();
    await u.click(tab(/^disponibilidad/i));
    expect(screen.getByRole('checkbox', { name: /mostrador/i })).toBeInTheDocument();
    expect(screen.getByText(/temporada/i)).toBeInTheDocument();
    expect(screen.queryByText('Pasa por cocina')).not.toBeInTheDocument();
    await u.click(tab(/^receta e inventario/i));
    expect(screen.getByRole('button', { name: /receta/i })).toBeInTheDocument();
    expect(screen.getByText('Vender sin existencia')).toBeInTheDocument();
    await u.click(tab(/^extras/i));
    expect(screen.getByText('Lista de grupos')).toBeInTheDocument();
    expect(screen.queryByText(/no dependen de «Guardar»/)).not.toBeInTheDocument();
  });

  it('cambiar de pestaña conserva lo capturado y Guardar manda todo', async () => {
    const u = userEvent.setup();
    const onClose = pinta();
    const nombre = await screen.findByLabelText('Nombre');
    await u.clear(nombre);
    await u.type(nombre, 'Horchata grande');
    await u.click(tab(/^disponibilidad/i));
    await u.click(screen.getByRole('checkbox', { name: /mostrador/i }));
    await u.click(tab(/^general/i));
    expect(screen.getByLabelText('Nombre')).toHaveValue('Horchata grande');
    await u.click(screen.getByRole('button', { name: /^guardar$/i }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(api.updateProduct.mock.calls[0][1]).toMatchObject({
      name: 'Horchata grande',
      sales: { counter: false, web: true, webPrice: null, allowOversell: true, description: '' },
    });
  });

  it('la casilla Web de Disponibilidad y el interruptor de Venta en web son el mismo dato', async () => {
    const u = userEvent.setup();
    pinta();
    await u.click(tab(/^disponibilidad/i));
    await u.click(screen.getByRole('checkbox', { name: /^web/i }));
    await u.click(tab(/^venta en web/i));
    expect(screen.getByLabelText(/se vende en la web/i)).not.toBeChecked();
    expect(screen.queryByLabelText(/precio en la web/i)).not.toBeInTheDocument();
  });

  it('el precio web arranca en «usar el de mostrador» con su cifra, y apagarlo lo vuelve obligatorio', async () => {
    const u = userEvent.setup();
    const onClose = pinta();
    await u.click(tab(/^venta en web/i));
    const usar = screen.getByRole('checkbox', { name: /usar el precio de mostrador \(\$35\.00\)/i });
    expect(usar).toBeChecked();
    await u.click(usar);
    await u.click(screen.getByRole('button', { name: /^guardar$/i }));
    expect(api.updateProduct).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    expect(await screen.findByText(/escribe el precio en la web/i)).toBeInTheDocument();
    await u.type(screen.getByLabelText(/precio en la web/i), '39');
    await u.click(screen.getByRole('button', { name: /^guardar$/i }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(sent().sales).toMatchObject({ web: true, webPrice: '39.00' });
  });

  it('apagar la web conserva el precio y la descripción capturados', async () => {
    const u = userEvent.setup();
    const onClose = pinta({ webPrice: '39.00', description: 'De arroz con canela' });
    await u.click(tab(/^venta en web/i));
    await u.click(screen.getByLabelText(/se vende en la web/i));
    await u.click(screen.getByRole('button', { name: /^guardar$/i }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(sent().sales).toMatchObject({ web: false, webPrice: '39.00', description: 'De arroz con canela' });
  });

  it('Guardar siempre se puede tocar: con un error lleva a su pestaña y la marca', async () => {
    const u = userEvent.setup();
    pinta();
    await u.click(tab(/^venta en web/i));
    await u.click(screen.getByRole('checkbox', { name: /usar el precio de mostrador/i }));
    await u.click(tab(/^extras/i));
    const guardar = screen.getByRole('button', { name: /^guardar$/i });
    expect(guardar).toBeEnabled();
    await u.click(guardar);
    expect(tab(/^venta en web/i)).toHaveAttribute('aria-selected', 'true');
    expect(within(tab(/^venta en web/i)).getByLabelText(/tiene un error/i)).toBeInTheDocument();
  });

  it('sin nada que medir en el almacén, Receta e inventario lo dice en vez de esconder la opción en silencio', async () => {
    const u = userEvent.setup();
    pinta({ measuresStock: false });
    await u.click(tab(/^receta e inventario/i));
    expect(screen.queryByText('Vender sin existencia')).not.toBeInTheDocument();
    expect(screen.getByText(/no descuenta del almacén/i)).toBeInTheDocument();
  });

  it('apagar «vender sin existencia» viaja en sales', async () => {
    const u = userEvent.setup();
    const onClose = pinta();
    await u.click(tab(/^receta e inventario/i));
    await u.click(screen.getByLabelText(/vender sin existencia/i));
    await u.click(screen.getByRole('button', { name: /^guardar$/i }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(sent().sales).toMatchObject({ allowOversell: false });
  });
});
