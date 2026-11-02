import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../../components/ui/provider';
import { ApiError } from '../../../api/client';
import { MergeSheet } from './MergeSheet';

const row = (id: number, name: string, unit = 'ml', kind = 'volumen', extra = {}) => ({
  id, name, isActive: true, baseUnitId: unit === 'ml' ? 5 : 8, baseUnitCode: unit, baseUnitKind: kind, currentCost: '0', minStock: null,
  isPrep: false, categoryId: null, category: null, countPlaceId: null, countPlace: null, onHand: '0', ...extra,
});

const back = vi.hoisted(() => ({
  articles: vi.fn(),
  similarArticles: vi.fn(),
  mergeArticles: vi.fn(),
}));
vi.mock('../../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

const leche = { ...row(1, 'Leche entera'), costSource: 'compra', updatedAt: '' };

const preview = (over = {}) => ({
  target: { id: 1, name: 'Leche entera', baseUnit: 'ml', factor: '1' },
  sources: [{ id: 2, name: 'Leche entera Marca B', baseUnit: 'ml', factor: '1' }],
  stock: [
    { branchId: 1, branch: 'Centro', ingredientId: 2, name: 'Leche entera Marca B', baseUnit: 'ml', before: '3000', after: '0', negativeAfter: false },
    { branchId: 1, branch: 'Centro', ingredientId: 1, name: 'Leche entera', baseUnit: 'ml', before: '8400', after: '11400', negativeAfter: false },
  ],
  recipes: [{ owner: 'Frappé de taro', sourceId: 2, beforeQty: '180', beforeUnit: 'ml', afterQty: '180', afterUnit: 'ml', merged: false }],
  presentations: [{ id: 9, sourceId: 2, name: 'Caja 12 × 1 L', brand: 'Marca B', qtyInBase: '12000', baseUnit: 'ml', action: 'move' }],
  supplierItems: 1, purchaseFormats: 0, costBefore: '0.023', costAfter: '0.0215', minStockBefore: null, minStockAfter: null,
  fingerprint: 'huella-1',
  ...over,
});

function montar(props: Partial<Parameters<typeof MergeSheet>[0]> = {}) {
  const onDone = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}><Provider>
      <MergeSheet target={leche} onClose={vi.fn()} onDone={onDone} {...props} />
    </Provider></QueryClientProvider>,
  );
  return onDone;
}

// Maqueta «Unir insumos», opción A: desde la ficha del que se queda, con los parecidos arriba.
describe('Unir insumos', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    back.articles.mockResolvedValue([
      row(1, 'Leche entera'), row(2, 'Leche entera Marca B'), row(3, 'Leche deslactosada'),
      row(4, 'Leche evaporada', 'pieza', 'pieza'), row(5, 'Base de chai', 'ml', 'volumen', { isPrep: true }),
      row(6, 'Leche de almendra'), row(7, 'Leche de coco'),
    ]);
    back.similarArticles.mockResolvedValue({ items: [
      { id: 2, name: 'Leche entera Marca B', baseUnitCode: 'ml', onHand: '3000' },
      { id: 3, name: 'Leche deslactosada', baseUnitCode: 'ml', onHand: '2000' },
      { id: 6, name: 'Leche de almendra', baseUnitCode: 'ml', onHand: '0' },
      { id: 7, name: 'Leche de coco', baseUnitCode: 'ml', onHand: '0' },
    ] });
    back.mergeArticles.mockImplementation((_id: number, _b: unknown, isPreview: boolean) =>
      Promise.resolve(isPreview ? preview() : preview({ fingerprint: '' })));
  });

  it('ofrece hasta 3 parecidos y «Ver todos»; sin el destino ni los preparados', async () => {
    montar();
    const similar = await screen.findByRole('group', { name: 'Parecidos' });
    expect(within(similar).getAllByRole('checkbox')).toHaveLength(3);
    fireEvent.click(screen.getByRole('button', { name: 'Ver todos los parecidos' }));
    expect(within(similar).getAllByRole('checkbox')).toHaveLength(4);
    const all = screen.getByRole('group', { name: 'Todos' });
    expect(within(all).queryByText('Leche entera', { exact: true })).toBeNull();
    expect(within(all).queryByText('Base de chai')).toBeNull();
  });

  it('el renglón entero marca y desmarca; «Siguiente» espera a que haya uno', async () => {
    montar();
    const next = await screen.findByRole('button', { name: 'Siguiente ›' });
    expect(next).toBeDisabled();
    const similar = await screen.findByRole('group', { name: 'Parecidos' });
    const b = within(similar).getByRole('checkbox', { name: /Leche entera Marca B/ });
    fireEvent.click(b);
    expect(b).toBeChecked();
    expect(next).not.toBeDisabled();
    fireEvent.click(b);
    expect(b).not.toBeChecked();
  });

  it('pide la vista previa, la pinta y confirma con su huella', async () => {
    const onDone = montar();
    const similar = await screen.findByRole('group', { name: 'Parecidos' });
    fireEvent.click(within(similar).getByRole('checkbox', { name: /Leche entera Marca B/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Siguiente ›' }));
    await waitFor(() => expect(back.mergeArticles).toHaveBeenCalledWith(1, { sources: [{ id: 2 }] }, true));
    expect(await screen.findByText('Frappé de taro')).toBeInTheDocument();
    expect(screen.getByText(/Caja 12 × 1 L · Marca B/)).toBeInTheDocument();
    expect(screen.getByText(/se archiva/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Unir' }));
    await waitFor(() => expect(back.mergeArticles).toHaveBeenLastCalledWith(1, { sources: [{ id: 2 }], fingerprint: 'huella-1' }, false));
    await waitFor(() => expect(onDone).toHaveBeenCalled());
  });

  it('«Volver» desde la vista previa conserva lo elegido', async () => {
    montar();
    const similar = await screen.findByRole('group', { name: 'Parecidos' });
    fireEvent.click(within(similar).getByRole('checkbox', { name: /Leche entera Marca B/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Siguiente ›' }));
    await screen.findByText('Frappé de taro');
    fireEvent.click(screen.getByRole('button', { name: 'Volver' }));
    expect(within(await screen.findByRole('group', { name: 'Parecidos' })).getByRole('checkbox', { name: /Leche entera Marca B/ })).toBeChecked();
  });

  it('con una venta en curso dice que vuelva a intentar y no pierde nada', async () => {
    back.mergeArticles.mockRejectedValueOnce(new ApiError(409, 'CONFLICT', 'hay una venta en curso con uno de esos insumos; vuelve a intentar en un momento', 'req'));
    montar();
    const similar = await screen.findByRole('group', { name: 'Parecidos' });
    fireEvent.click(within(similar).getByRole('checkbox', { name: /Leche entera Marca B/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Siguiente ›' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/vuelve a intentar/);
    expect(within(screen.getByRole('group', { name: 'Parecidos' })).getByRole('checkbox', { name: /Leche entera Marca B/ })).toBeChecked();
  });

  it('si algo cambió al confirmar, avisa y vuelve a pedir la vista previa', async () => {
    back.mergeArticles
      .mockResolvedValueOnce(preview())
      .mockRejectedValueOnce(new ApiError(409, 'MERGE_STALE', 'algo cambió mientras revisabas', 'req'))
      .mockResolvedValueOnce(preview({ fingerprint: 'huella-2' }));
    const onDone = montar();
    const similar = await screen.findByRole('group', { name: 'Parecidos' });
    fireEvent.click(within(similar).getByRole('checkbox', { name: /Leche entera Marca B/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Siguiente ›' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Unir' }));
    expect(await screen.findByText(/Algo cambió mientras revisabas/)).toBeInTheDocument();
    await waitFor(() => expect(back.mergeArticles).toHaveBeenCalledTimes(3));
    expect(onDone).not.toHaveBeenCalled();
  });

  it('una presentación que choca bloquea «Unir» y dice por qué', async () => {
    back.mergeArticles.mockResolvedValue(preview({ presentations: [
      { id: 9, sourceId: 2, name: 'Caja', brand: 'Lala', qtyInBase: '8000', baseUnit: 'ml', action: 'conflict' },
    ] }));
    montar();
    const similar = await screen.findByRole('group', { name: 'Parecidos' });
    fireEvent.click(within(similar).getByRole('checkbox', { name: /Leche entera Marca B/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Siguiente ›' }));
    expect(await screen.findByText(/se llama igual que una de Leche entera y trae distinto/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Unir' })).toBeDisabled();
  });

  it('un insumo medido distinto pide su equivalencia antes de la vista previa', async () => {
    montar();
    const similar = await screen.findByRole('group', { name: 'Parecidos' });
    fireEvent.click(screen.getByRole('button', { name: 'Ver todos los parecidos' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Buscar otro insumo' }), { target: { value: 'evap' } });
    fireEvent.click(within(screen.getByRole('group', { name: 'Todos' })).getByRole('checkbox', { name: /Leche evaporada/ }));
    expect(similar).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Siguiente ›' }));
    expect(await screen.findByText('¿Cuánto equivale?')).toBeInTheDocument();
    const ver = screen.getByRole('button', { name: 'Ver qué cambia ›' });
    expect(ver).toBeDisabled();
    const box = screen.getByLabelText('1 pieza de Leche evaporada en ml de Leche entera');
    fireEvent.click(box);
    fireEvent.click(screen.getByRole('button', { name: '3' }));
    fireEvent.click(screen.getByRole('button', { name: '6' }));
    fireEvent.click(screen.getByRole('button', { name: '0' }));
    expect(ver).not.toBeDisabled();
    fireEvent.click(ver);
    await waitFor(() => expect(back.mergeArticles).toHaveBeenCalledWith(1, { sources: [{ id: 4, factor: '360' }] }, true));
  });

  it('abierta desde un grupo de duplicados, llega con los demás marcados y desmarcables', async () => {
    montar({ initialSources: [2, 3] });
    const similar = await screen.findByRole('group', { name: 'Parecidos' });
    expect(within(similar).getByRole('checkbox', { name: /Leche entera Marca B/ })).toBeChecked();
    fireEvent.click(within(similar).getByRole('checkbox', { name: /Leche deslactosada/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Siguiente ›' }));
    await waitFor(() => expect(back.mergeArticles).toHaveBeenCalledWith(1, { sources: [{ id: 2 }] }, true));
  });
});
