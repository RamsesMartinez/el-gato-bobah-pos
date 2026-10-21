import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, within, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { useSessionStore } from '../../../stores/session';
import { Provider } from '../../../components/ui/provider';
import { ApiError } from '../../../api/client';
import { NewArticleSheet } from './NewArticleSheet';
import { ArchiveSheet } from './ArchiveSheet';
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
  countPlaces: vi.fn(() => Promise.resolve([] as unknown[])),
  createCountPlace: vi.fn(),
  createArticle: vi.fn(),
  reactivateArticle: vi.fn(() => Promise.resolve()),
  article: vi.fn(),
  updateArticle: vi.fn(() => Promise.resolve({ updatedAt: 'v2' })),
  articleCategories: vi.fn(() => Promise.resolve([])),
  presentations: vi.fn(() => Promise.resolve([{ id: 3, ingredientId: 7, name: 'Bolsa', qtyInBase: '5000', baseUnitCode: 'g', active: true, approximate: false, purchaseUnitId: null }])),
  articleCostHistory: vi.fn(),
  articleAudit: vi.fn(),
  correctArticleCost: vi.fn(() => Promise.resolve()),
  presentationSuggestions: vi.fn(() => Promise.resolve([])),
  articleArchiveCheck: vi.fn(),
  archiveArticle: vi.fn(() => Promise.resolve()),
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

  // La prueba sin contexto tocó «Guardar» antes de que llegaran las unidades: el botón seguía
  // apagado sin decir por qué y el toque se perdía.
  it('Guardar funciona aunque las unidades todavía no hayan llegado', async () => {
    let soltar: (v: unknown) => void = () => {};
    const unidades = back.units.getMockImplementation()!;
    back.units.mockImplementationOnce(() => new Promise((r) => { soltar = r; }) as never);
    back.createArticle.mockResolvedValue({ id: 9 });
    montar(<NewArticleSheet onClose={vi.fn()} onCreated={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText(/Nombre/), { target: { value: 'Leche entera' } });
    fireEvent.click(screen.getByRole('button', { name: 'Líquido' }));
    const guardar = screen.getByRole('button', { name: 'Guardar' });
    expect(guardar).not.toBeDisabled();
    fireEvent.click(guardar);
    soltar(await unidades());
    await waitFor(() => expect(back.createArticle).toHaveBeenCalledWith(expect.objectContaining({ name: 'Leche entera', baseUnitId: 3 })));
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

  // Spec 043: la ficha guarda sola, campo por campo. Sin «Guardar» y sin preguntar al salir si todo
  // quedó guardado (#119).
  const minimo = () => within(screen.getByText('Mínimo', { exact: true }).parentElement!).getByRole('textbox');

  it('salir del campo guarda solo ese campo con la versión que se vio; no hay «Guardar»', async () => {
    back.updateArticle.mockResolvedValue({ updatedAt: 'v2' });
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    const nombre = await screen.findByDisplayValue('Hielo en bolsa');
    expect(screen.queryByRole('button', { name: 'Guardar' })).toBeNull();
    fireEvent.change(nombre, { target: { value: 'Hielo' } });
    expect(screen.getByLabelText('Cambios sin guardar')).toBeInTheDocument();
    fireEvent.blur(nombre);
    // La ficha completa, con lo último guardado en lo que no cambió: la API anterior (durante una
    // subida) toma un campo ausente como vacío y borraría la categoría, la ubicación y el mínimo.
    await waitFor(() => expect(back.updateArticle).toHaveBeenCalledWith(7, {
      expectedUpdatedAt: '2026-10-19T10:00:00.123456Z', name: 'Hielo', categoryId: null, placeId: null, minStock: '10000',
    }));
    expect(await screen.findByText('Guardado ✓', {}, { timeout: 3000 })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Guardar' })).toBeNull();
  });

  it('Enter en el mínimo lo guarda en la unidad base; cambiar g/kg no cambia la cantidad', async () => {
    back.updateArticle.mockResolvedValue({ updatedAt: 'v2' });
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    await screen.findByDisplayValue('Hielo en bolsa');
    expect(minimo()).toHaveValue('10');
    fireEvent.click(screen.getByRole('button', { name: /^g$/ }));
    expect(minimo()).toHaveValue('10000');
    fireEvent.click(screen.getByRole('button', { name: /^kg$/ }));
    expect(minimo()).toHaveValue('10');
    expect(back.updateArticle).not.toHaveBeenCalled();
    fireEvent.change(minimo(), { target: { value: '12' } });
    fireEvent.keyDown(minimo(), { key: 'Enter' });
    await waitFor(() => expect(back.updateArticle).toHaveBeenCalledWith(7, expect.objectContaining({ minStock: '12000' })));
  });

  it('mientras guarda, ningún campo se deshabilita', async () => {
    back.updateArticle.mockImplementation(() => new Promise(() => {}));
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    const nombre = await screen.findByDisplayValue('Hielo en bolsa');
    fireEvent.change(nombre, { target: { value: 'Hielo' } });
    fireEvent.blur(nombre);
    expect(await screen.findByText('Guardando…')).toBeInTheDocument();
    expect(nombre).not.toBeDisabled();
    expect(minimo()).not.toBeDisabled();
  });

  it('sin red conserva lo escrito y ofrece reintentar; salir así pregunta antes', async () => {
    back.updateArticle.mockRejectedValueOnce(new TypeError('Failed to fetch')).mockResolvedValueOnce({ updatedAt: 'v2' });
    const onClose = vi.fn();
    montar(<ArticleSheet id={7} onClose={onClose} />);
    const nombre = await screen.findByDisplayValue('Hielo en bolsa');
    fireEvent.change(nombre, { target: { value: 'Hielo' } });
    fireEvent.blur(nombre);
    expect(await screen.findByText(/No se pudo guardar/)).toBeInTheDocument();
    expect(nombre).toHaveValue('Hielo');
    fireEvent.click(screen.getByRole('button', { name: '‹ Artículos' }));
    expect(await screen.findByText('¿Descartar cambios?')).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Seguir editando' }));
    await waitFor(() => expect(screen.queryByText('¿Descartar cambios?')).toBeNull(), { timeout: 3000 });
    fireEvent.click(await screen.findByRole('button', { name: 'Reintentar' }));
    await waitFor(() => expect(back.updateArticle).toHaveBeenCalledTimes(2));
    expect(back.updateArticle).toHaveBeenLastCalledWith(7, expect.objectContaining({ expectedUpdatedAt: '2026-10-19T10:00:00.123456Z', name: 'Hielo', minStock: '10000' }));
    expect(await screen.findByText('Guardado ✓', {}, { timeout: 3000 })).toBeInTheDocument();
  });

  it('si el servidor rechaza el nombre, el campo vuelve a lo guardado y dice por qué', async () => {
    back.updateArticle.mockRejectedValueOnce(new ApiError(409, 'NAME_TAKEN', 'ya hay un artículo que se llama así', 'r', { existingId: 4 }));
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    const nombre = await screen.findByDisplayValue('Hielo en bolsa');
    fireEvent.change(nombre, { target: { value: 'Azúcar' } });
    fireEvent.blur(nombre);
    await waitFor(() => expect(nombre).toHaveValue('Hielo en bolsa'), { timeout: 3000 });
  });

  it('un nombre vacío se marca, no se manda, y salir pregunta antes', async () => {
    const onClose = vi.fn();
    montar(<ArticleSheet id={7} onClose={onClose} />);
    const nombre = await screen.findByDisplayValue('Hielo en bolsa');
    fireEvent.change(nombre, { target: { value: '   ' } });
    fireEvent.blur(nombre);
    expect(await screen.findByText('El nombre no puede quedar vacío.')).toBeInTheDocument();
    expect(back.updateArticle).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: '‹ Artículos' }));
    expect(await screen.findByText('¿Descartar cambios?')).toBeInTheDocument();
  });

  // Durante una subida azul/verde la API anterior responde 204 sin la versión nueva: la ficha la pide
  // para que el guardado siguiente no choque contra el anterior.
  it('si la respuesta no trae la versión, la pide antes del guardado siguiente', async () => {
    back.updateArticle.mockResolvedValueOnce(undefined as never).mockResolvedValueOnce({ updatedAt: 'v3' });
    back.article.mockResolvedValueOnce(ficha).mockResolvedValue({ ...ficha, name: 'Hielo', updatedAt: 'v-servidor' });
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    const nombre = await screen.findByDisplayValue('Hielo en bolsa');
    fireEvent.change(nombre, { target: { value: 'Hielo' } });
    fireEvent.blur(nombre);
    await screen.findByText('Guardado ✓', {}, { timeout: 3000 });
    fireEvent.change(minimo(), { target: { value: '12' } });
    fireEvent.blur(minimo());
    await waitFor(() => expect(back.updateArticle).toHaveBeenLastCalledWith(7, expect.objectContaining({ expectedUpdatedAt: 'v-servidor', minStock: '12000' })), { timeout: 3000 });
  });

  // Probado en local: tras guardar el nombre, cerrar y volver a abrir la ficha mostraba el nombre
  // anterior. El formulario se llenaba con la ficha guardada en caché antes de que llegara la nueva.
  it('al volver a abrir la ficha muestra lo último guardado, no la caché vieja', async () => {
    let actual = { ...ficha };
    back.article.mockImplementation(() => new Promise((r) => setTimeout(() => r(actual), 20)));
    back.updateArticle.mockImplementation(((_id: number, b: { name?: string }) => { actual = { ...actual, name: b.name ?? actual.name, updatedAt: 'v2' }; return Promise.resolve({ updatedAt: 'v2' }); }) as never);
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const abrir = () => render(<QueryClientProvider client={qc}><Provider><ArticleSheet id={7} onClose={vi.fn()} /></Provider></QueryClientProvider>);
    const primera = abrir();
    const nombre = await screen.findByDisplayValue('Hielo en bolsa');
    fireEvent.change(nombre, { target: { value: 'Hielo picado' } });
    fireEvent.blur(nombre);
    await screen.findByText('Guardado ✓', {}, { timeout: 3000 });
    primera.unmount();
    abrir();
    expect(await screen.findByDisplayValue('Hielo picado', {}, { timeout: 3000 })).toBeInTheDocument();
    expect(screen.queryByDisplayValue('Hielo en bolsa')).toBeNull();
  });

  // Spec 043, FR-001 y FR-003: «Ubicación» se elige o se crea en la ficha y se guarda al elegirla.
  it('«Ubicación» guarda al elegir y crea una nueva desde el buscador', async () => {
    back.countPlaces.mockResolvedValue([{ id: 1, name: 'Refri', position: 0, ingredients: 1 }]);
    back.createCountPlace.mockResolvedValue({ id: 5 });
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    await screen.findByDisplayValue('Hielo en bolsa');
    expect(screen.getByText('Ubicación')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Sin definir/ }));
    fireEvent.click(await screen.findByRole('button', { name: 'Refri' }));
    await waitFor(() => expect(back.updateArticle).toHaveBeenCalledWith(7, expect.objectContaining({ placeId: 1 })));
    fireEvent.click(await screen.findByRole('button', { name: /Refri/ }));
    fireEvent.change(await screen.findByPlaceholderText('Buscar…'), { target: { value: 'Bodega' } });
    fireEvent.click(await screen.findByRole('button', { name: /Crear «Bodega»/ }));
    await waitFor(() => expect(back.updateArticle).toHaveBeenLastCalledWith(7, expect.objectContaining({ placeId: 5 })), { timeout: 3000 });
  });

  // En la tableta, tocar un botón no siempre quita el foco del campo: salir guarda primero lo que
  // tiene el campo enfocado y espera a que termine (R2).
  it('salir con el foco en un campo cambiado lo guarda y después regresa, sin preguntar', async () => {
    back.updateArticle.mockResolvedValue({ updatedAt: 'v2' });
    const onClose = vi.fn();
    montar(<ArticleSheet id={7} onClose={onClose} />);
    const nombre = await screen.findByDisplayValue('Hielo en bolsa');
    nombre.focus();
    fireEvent.change(nombre, { target: { value: 'Hielo picado' } });
    fireEvent.click(screen.getByRole('button', { name: '‹ Artículos' }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(back.updateArticle).toHaveBeenCalledWith(7, expect.objectContaining({ name: 'Hielo picado' }));
    expect(screen.queryByText('¿Descartar cambios?')).toBeNull();
  });

  // #120 y #119: tras guardar, la ficha se volvía a llenar con los datos viejos de la caché. El mínimo
  // aparecía vacío, el guardado siguiente lo borraba, y salir preguntaba aunque todo estaba guardado.
  it('tras guardar, la ficha conserva lo guardado y salir no pregunta (#120, #119)', async () => {
    back.article.mockResolvedValue({ ...ficha, minStock: null });
    back.updateArticle.mockResolvedValue({ updatedAt: 'v2' });
    const onClose = vi.fn();
    montar(<ArticleSheet id={7} onClose={onClose} />);
    await screen.findByDisplayValue('Hielo en bolsa');
    fireEvent.click(screen.getByRole('button', { name: /^g$/ }));
    fireEvent.change(minimo(), { target: { value: '750' } });
    fireEvent.blur(minimo());
    await waitFor(() => expect(back.updateArticle).toHaveBeenCalledWith(7, expect.objectContaining({ minStock: '750' })));
    await screen.findByText('Guardado ✓', {}, { timeout: 3000 });
    await new Promise((r) => setTimeout(r, 60));
    expect(minimo(), 'el mínimo volvió al valor viejo tras guardar').toHaveValue('750');
    fireEvent.click(screen.getByRole('button', { name: '‹ Artículos' }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(screen.queryByText('¿Descartar cambios?')).toBeNull();
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

// US4 (FR-007): una receta activa impide archivar, pero no es un callejón: cada una se abre desde aquí.
describe('Archivar', () => {
  it('con una receta activa no deja archivar y ofrece abrir esa receta', async () => {
    back.articleArchiveCheck.mockResolvedValue({ activeRecipes: [{ kind: 'product', id: 5, name: 'Frappé de mango' }], onHand: [], pendingReceipts: 0, openCount: false });
    montar(<ArchiveSheet article={ficha as never} onClose={vi.fn()} onDone={vi.fn()} />);
    expect(await screen.findByRole('button', { name: /Frappé de mango/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Archivar' })).toBeDisabled();
  });

  it('sin bloqueos, muestra lo que queda en existencia y archiva al confirmar', async () => {
    back.articleArchiveCheck.mockResolvedValue({ activeRecipes: [], onHand: [{ branch: 'Matriz', qty: '27000' }], pendingReceipts: 0, openCount: false });
    const onDone = vi.fn();
    montar(<ArchiveSheet article={ficha as never} onClose={vi.fn()} onDone={onDone} />);
    expect(await screen.findByText(/en Matriz/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Archivar' }));
    await waitFor(() => expect(back.archiveArticle).toHaveBeenCalledWith(7));
    await waitFor(() => expect(onDone).toHaveBeenCalled());
  });
});
