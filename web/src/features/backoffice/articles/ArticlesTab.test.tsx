import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { useSessionStore } from '../../../stores/session';
import { Provider } from '../../../components/ui/provider';
import { ArticlesTab } from './ArticlesTab';

const row = (id: number, name: string, extra: object = {}) => ({
  id, name, isActive: true, baseUnitId: 1, baseUnitCode: 'g', baseUnitKind: 'masa', currentCost: '0.006', minStock: null,
  isPrep: false, categoryId: null, category: null, countPlaceId: null, countPlace: null, onHand: '27000', presentations: [], ...extra,
});
const back = vi.hoisted(() => ({
  articles: vi.fn(),
  countPlaces: vi.fn(() => Promise.resolve([{ id: 4, name: 'Refri', position: 0, ingredients: 1 }])),
  units: vi.fn(() => Promise.resolve({ items: [] })),
}));
vi.mock('../../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

function montar(onOpen = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><ArticlesTab onOpen={onOpen} /></Provider></QueryClientProvider>);
  return onOpen;
}

// US1 (FR-001): encontrar un artículo entre cientos sin bajar a mano.
describe('Almacén › Artículos', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'cajero', permissions: ['inventory.view', 'inventory.configure'] } as never });
    back.articles.mockImplementation(() => Promise.resolve([
      row(1, 'Hielo en bolsa', { countPlace: 'Refri', countPlaceId: 4, presentations: [{ id: 9, name: 'Bolsa', qtyInBase: '5000' }] }),
      row(2, 'Azúcar', { onHand: '-250', currentCost: '0' }),
    ]));
  });

  it('pinta cada artículo con su existencia, su costo y sus empaques en la unidad que se dice', async () => {
    montar();
    expect(await screen.findByText('Hielo en bolsa')).toBeTruthy();
    expect(screen.getByText('27 kg')).toBeTruthy();
    expect(screen.getByText('$6.00 / kg')).toBeTruthy();
    expect(screen.getByText('Bolsa · 5 kg')).toBeTruthy();
    expect(screen.getByText('Sin costo', { selector: 'span' })).toBeTruthy();
    expect(screen.getByText('-250 g')).toBeTruthy();
  });

  it('escribir en el buscador pide la lista filtrada al servidor', async () => {
    montar();
    await screen.findByText('Hielo en bolsa');
    fireEvent.change(screen.getByLabelText('Buscar artículo o empaque'), { target: { value: 'hie' } });
    await waitFor(() => expect(back.articles).toHaveBeenLastCalledWith(expect.objectContaining({ q: 'hie' })));
  });

  it('los filtros son de un toque y se ve cuál está puesto', async () => {
    montar();
    await screen.findByText('Hielo en bolsa');
    const sinCosto = screen.getByRole('button', { name: 'Sin costo' });
    fireEvent.click(sinCosto);
    await waitFor(() => expect(back.articles).toHaveBeenLastCalledWith(expect.objectContaining({ noCost: true })));
    expect(sinCosto.getAttribute('aria-pressed')).toBe('true');
    fireEvent.click(screen.getByRole('button', { name: 'Archivados' }));
    await waitFor(() => expect(back.articles).toHaveBeenLastCalledWith(expect.objectContaining({ archived: true })));
  });

  it('una receta estimada se marca «por revisar» en la lista', async () => {
    back.articles.mockImplementation(() => Promise.resolve([row(1, 'Jarabe', { compositionStatus: 'estimated' })]));
    montar();
    expect(await screen.findByText('por revisar')).toBeTruthy();
  });

  it('tocar un renglón abre su ficha', async () => {
    const onOpen = montar();
    fireEvent.click(await screen.findByRole('button', { name: /Hielo en bolsa/ }));
    expect(onOpen).toHaveBeenCalledWith(1);
  });

  it('sin resultados dice qué hacer', async () => {
    back.articles.mockImplementation(() => Promise.resolve([]));
    montar();
    expect(await screen.findByText(/Nada coincide/)).toBeTruthy();
  });

  it('«+ Nuevo» solo se ofrece a quien configura el almacén', async () => {
    useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'admin', permissions: ['inventory.view'] } as never });
    montar();
    await screen.findByText('Hielo en bolsa');
    expect(screen.queryByRole('button', { name: /Nuevo/ })).toBeNull();
  });

  it('los controles tocables miden al menos 44 px', async () => {
    montar();
    await screen.findByText('Hielo en bolsa');
    for (const name of ['Sin costo', 'Existencia negativa', 'Activos', 'Archivados']) {
      expect(getComputedStyle(screen.getByRole('button', { name })).minHeight).toBe('44px');
    }
  });
});
