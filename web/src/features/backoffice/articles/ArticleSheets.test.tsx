import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { useSessionStore } from '../../../stores/session';
import { Provider } from '../../../components/ui/provider';
import { ApiError } from '../../../api/client';
import { NewArticleSheet } from './NewArticleSheet';
import { ArticleSheet } from './ArticleSheet';
import { ArticleAuditSheet } from './ArticleAuditSheet';
import { whatChanged } from './articleAudit';

const ficha = {
  id: 7, name: 'Hielo en bolsa', isActive: true, baseUnitId: 1, baseUnitCode: 'g', baseUnitKind: 'masa', currentCost: '0.006',
  minStock: '10000', isPrep: false, categoryId: null, category: null, countPlaceId: null, countPlace: null, onHand: '27000',
  presentations: [{ id: 3, name: 'Bolsa', qtyInBase: '5000' }], costSource: 'compra', updatedAt: '2026-10-19T10:00:00.123456Z',
};
const compra = (id: number, cost: string) => ({ kind: 'purchase', at: `2026-10-${String(id).padStart(2, '0')}T10:00:00Z`, id, unitCost: cost, presentation: 'Bolsa', packQty: '5000', packCost: String(Number(cost) * 5000), supplier: 'Proveedor A', user: null });
const back = vi.hoisted(() => ({
  units: vi.fn(() => Promise.resolve({ items: [
    { id: 1, code: 'g', name: 'Gramo', kind: 'masa', toBase: '1' }, { id: 2, code: 'kg', name: 'Kilo', kind: 'masa', toBase: '1000' },
    { id: 3, code: 'ml', name: 'Mililitro', kind: 'volumen', toBase: '1' },
  ] })),
  countPlaces: vi.fn(() => Promise.resolve([])),
  createArticle: vi.fn(),
  reactivateArticle: vi.fn(() => Promise.resolve()),
  article: vi.fn(),
  updateArticle: vi.fn(() => Promise.resolve()),
  articleCategories: vi.fn(() => Promise.resolve([])),
  presentations: vi.fn(() => Promise.resolve([{ id: 3, ingredientId: 7, name: 'Bolsa', qtyInBase: '5000', baseUnitCode: 'g', active: true, approximate: false, purchaseUnitId: null }])),
  articleCostHistory: vi.fn(),
  articleAudit: vi.fn(),
  correctArticleCost: vi.fn(() => Promise.resolve()),
  presentationSuggestions: vi.fn(() => Promise.resolve([])),
}));
vi.mock('../../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
const pos = vi.hoisted(() => ({ businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' })) }));
vi.mock('../../../api/pos', () => ({ posApi: pos }));

function montar(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider>{ui}</Provider></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'cajero', permissions: ['inventory.view', 'inventory.configure', 'inventory.receive'] } as never });
  back.article.mockResolvedValue(ficha);
  back.articleCostHistory.mockResolvedValue({ items: [compra(18, '0.006'), compra(10, '0.0055')], next: 'cursor-1', trendPct: '9.1' });
  back.articleAudit.mockResolvedValue({ items: [], next: null });
});

// US2 (FR-002): la hoja corta con lo obligatorio marcado; cancelar no llama al servidor.
describe('Nuevo artículo', () => {
  it('Guardar se habilita con nombre y en qué se mide, y manda la existencia en la unidad base', async () => {
    back.createArticle.mockResolvedValue({ id: 9 });
    const onCreated = vi.fn();
    montar(<NewArticleSheet onClose={vi.fn()} onCreated={onCreated} />);
    const guardar = await screen.findByRole('button', { name: 'Guardar' });
    expect(guardar).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/Nombre/), { target: { value: 'Leche entera' } });
    fireEvent.click(screen.getByRole('button', { name: 'Líquido' }));
    fireEvent.change(await screen.findByLabelText(/Existencia inicial/), { target: { value: '2' } });
    await waitFor(() => expect(guardar).not.toBeDisabled());
    fireEvent.click(guardar);
    await waitFor(() => expect(back.createArticle).toHaveBeenCalledWith(expect.objectContaining({ name: 'Leche entera', baseUnitId: 3, initialStock: '2000' })));
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(9));
  });

  it('cancelar no llama al servidor', async () => {
    const onClose = vi.fn();
    montar(<NewArticleSheet onClose={onClose} onCreated={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText(/Nombre/), { target: { value: 'Algo' } });
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }));
    expect(onClose).toHaveBeenCalled();
    expect(back.createArticle).not.toHaveBeenCalled();
  });

  it('si el nombre ya existe archivado, ofrece reactivarlo', async () => {
    back.createArticle.mockRejectedValue(new ApiError(409, 'NAME_TAKEN', 'ya hay un artículo que se llama así', 'r', { existingId: 4, archived: true }));
    const onCreated = vi.fn();
    montar(<NewArticleSheet onClose={vi.fn()} onCreated={onCreated} />);
    fireEvent.change(await screen.findByLabelText(/Nombre/), { target: { value: 'Cajeta' } });
    fireEvent.click(screen.getByRole('button', { name: 'Líquido' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Guardar' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Reactivar' }));
    await waitFor(() => expect(back.reactivateArticle).toHaveBeenCalledWith(4));
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(4));
  });
});

// US3 (FR-004 a FR-006): la ficha, su historial y su tendencia.
describe('Ficha del artículo', () => {
  it('pinta el costo por kg, la tendencia que da el servidor y lo pagado por empaque', async () => {
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    expect(await screen.findByText('▲ Sube 9.1 %')).toBeInTheDocument();
    expect(screen.getAllByText('$6.00 / kg').length).toBeGreaterThan(0);
    expect(screen.getAllByText(/\$30\.00 por Bolsa/).length).toBe(1);
  });

  it('«Ver más» pide la página siguiente', async () => {
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Ver más' }));
    await waitFor(() => expect(back.articleCostHistory).toHaveBeenCalledWith(7, 'cursor-1'));
  });

  it('sin la segunda compra no hay indicador de tendencia', async () => {
    back.articleCostHistory.mockResolvedValue({ items: [compra(18, '0.006')], next: null, trendPct: null });
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    await screen.findByText(/Corregir costo/);
    expect(screen.queryByText(/Sube|Baja/)).toBeNull();
  });

  it('un preparado no ofrece corregir el costo', async () => {
    back.article.mockResolvedValue({ ...ficha, isPrep: true });
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    expect(await screen.findByText('El costo de un preparado sale de su receta.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Corregir costo' })).toBeNull();
  });

  it('guardar manda la versión que se vio; salir con cambios pregunta antes', async () => {
    const onClose = vi.fn();
    montar(<ArticleSheet id={7} onClose={onClose} />);
    const nombre = await screen.findByDisplayValue('Hielo en bolsa');
    fireEvent.change(nombre, { target: { value: 'Hielo' } });
    fireEvent.click(screen.getByRole('button', { name: '‹ Artículos' }));
    expect(await screen.findByText('¿Descartar cambios?')).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Seguir editando' }));
    await waitFor(() => expect(screen.queryByText('¿Descartar cambios?')).toBeNull());
    fireEvent.click(await screen.findByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(back.updateArticle).toHaveBeenCalledWith(7, expect.objectContaining({
      expectedUpdatedAt: '2026-10-19T10:00:00.123456Z', name: 'Hielo', minStock: '10000',
    })));
  });

  // Prueba sin contexto (2026-10-19): «Guardar» activo después de archivar o corregir el costo hacía
  // dudar si faltaba confirmar algo.
  it('«Guardar» solo aparece cuando hay cambios sin guardar', async () => {
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    const nombre = await screen.findByDisplayValue('Hielo en bolsa');
    expect(screen.queryByRole('button', { name: 'Guardar' })).toBeNull();
    fireEvent.change(nombre, { target: { value: 'Hielo' } });
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeInTheDocument();
  });

  // Con un solo registro, la gráfica pintaba en el eje cifras que nadie capturó ($23.10 y $20.90 para
  // una corrección de $22) y decía «compra» a una corrección.
  it('con un solo registro no inventa cifras en el eje ni lo llama compra', async () => {
    back.articleCostHistory.mockResolvedValue({ items: [{ kind: 'correction', at: '2026-10-19T10:00:00Z', id: 1, unitCost: '0.022',
      presentation: null, packQty: null, packCost: null, supplier: null, user: 'Ana' }], next: null, trendPct: null });
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    await screen.findByText(/Capturado a mano/);
    expect(screen.queryByText('$23.10')).toBeNull();
    expect(screen.queryByText('$20.90')).toBeNull();
    expect(screen.queryByText(/sola compra/)).toBeNull();
  });

  it('«+ Agregar empaque» abre directo el empaque nuevo', async () => {
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar empaque' }));
    expect(await screen.findByText('Nuevo empaque')).toBeInTheDocument();
  });

  it('la ficha dice cuánto hay, sin repetir la unidad de las piezas', async () => {
    back.article.mockResolvedValue({ ...ficha, baseUnitCode: 'pieza', baseUnitKind: 'pieza', onHand: '12' });
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    expect(await screen.findByText(/Hay 12 piezas/)).toBeInTheDocument();
    expect(screen.queryByText(/piezas \(piezas\)/)).toBeNull();
  });

  // Revisión de código: el renglón de un empaque no se podía tocar para editarlo.
  it('tocar un empaque lo abre para editarlo', async () => {
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    fireEvent.click(await screen.findByRole('button', { name: /Bolsa · 5 kg/ }));
    expect(await screen.findByText('Cambiar Bolsa')).toBeInTheDocument();
  });

  it('una receta estimada dice «por revisar»', async () => {
    back.article.mockResolvedValue({ ...ficha, compositionStatus: 'estimated' });
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    expect(await screen.findByText('por revisar')).toBeInTheDocument();
  });

  it('recién creado, «Agregar empaque» va primero', async () => {
    montar(<ArticleSheet id={7} justCreated onClose={vi.fn()} />);
    const agregar = await screen.findByRole('button', { name: '+ Agregar empaque' });
    const nombre = screen.getByDisplayValue('Hielo en bolsa');
    expect(agregar.compareDocumentPosition(nombre) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});

// FR-016: la bitácora del artículo es de solo lectura y dice quién y qué cambió.
describe('Bitácora del artículo', () => {
  it('pinta quién, cuándo y qué cambió, y pide más de 20 en 20', async () => {
    back.articleAudit.mockResolvedValueOnce({
      items: [{ id: 1, at: '2026-10-19T21:14:00Z', action: 'ingredient.cost_corrected', entity: 'ingredient', actor: 'Ana', before: { cost: '0.03' }, after: { cost: '0.006' } }],
      next: 'c2',
    });
    montar(<ArticleAuditSheet id={7} name="Hielo en bolsa" baseUnitCode="g" onClose={vi.fn()} />);
    expect(await screen.findByText('Ana')).toBeInTheDocument();
    expect(screen.getByText('Costo: $30.00 / kg → $6.00 / kg')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Ver más' }));
    await waitFor(() => expect(back.articleAudit).toHaveBeenCalledWith(7, 'c2'));
  });

  it('whatChanged solo habla de lo que quien opera reconoce', () => {
    expect(whatChanged({ before: { name: 'A', placeId: 3 }, after: { name: 'B', placeId: 4 } }, 'ml')).toBe('Nombre: A → B');
  });

  // Prueba de pantalla (2026-10-19): la bitácora decía «Costo: 0.024 → 0.026», el precio por ml.
  it('el costo y el mínimo se dicen en la unidad en que se compra', () => {
    expect(whatChanged({ before: { cost: '0.024' }, after: { cost: '0.026' } }, 'ml')).toBe('Costo: $24.00 / L → $26.00 / L');
    expect(whatChanged({ before: { minStock: '5000' }, after: { minStock: '10000' } }, 'g')).toBe('Mínimo: 5 kg → 10 kg');
  });

  it('sin «ver el almacén» la ficha no ofrece la bitácora', async () => {
    useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'admin', permissions: [] } as never });
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    await screen.findByDisplayValue('Hielo en bolsa');
    expect(screen.queryByRole('button', { name: 'Bitácora' })).toBeNull();
  });
});
