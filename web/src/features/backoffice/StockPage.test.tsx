import { describe, it, expect, vi, beforeEach } from 'vitest';
import { cleanup, render, screen, fireEvent, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';

import { useSessionStore } from '../../stores/session';

import { Provider } from '../../components/ui/provider';
import { StockPage } from './StockPage';

const oneWarehouse = { items: [{ id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 2 }] };
const back = vi.hoisted(() => ({
  createTransfer: vi.fn((_b?: unknown) => Promise.resolve({ id: 1 })),
  countPlaces: vi.fn(() => Promise.resolve([{ id: 4, name: 'Barra', position: 0, ingredients: 1 }])),
  stockLevels: vi.fn((_w?: number | 'all') => Promise.resolve({ items: [] as unknown[] })),
  expirySummary: vi.fn((_w?: number | 'all') => Promise.resolve({ expired: 0, within7: 0 })),
  expiries: vi.fn(() => Promise.resolve({ today: '2026-11-23', counts: { expired: 0, within7: 0, within30: 0, within60: 0 }, lots: [] })),
  warehouses: vi.fn(() => Promise.resolve({ items: [{ id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 2 }] })),
  stockMovements: vi.fn(() => Promise.resolve({ items: [] })),
  units: vi.fn(() => Promise.resolve({ items: [
    { id: 3, code: 'ml', name: 'Mililitro', kind: 'volumen', toBase: '1' },
    { id: 7, code: 'l', name: 'Litro', kind: 'volumen', toBase: '1000' },
  ] })),
  presentations: vi.fn(() => Promise.resolve([
    { id: 5, ingredientId: 2, name: 'Botella', qtyInBase: '750', baseUnitCode: 'ml', active: true, approximate: false, purchaseUnitId: null },
    { id: 6, ingredientId: 2, name: 'Garrafa', qtyInBase: '5000', baseUnitCode: 'ml', active: false, approximate: false, purchaseUnitId: null },
  ])),
  ingredients: vi.fn(() => Promise.resolve({
    items: [
      { id: 1, name: 'Azúcar', onHand: '1200', baseUnitCode: 'g', baseUnitKind: 'masa', baseUnitId: 1, isPrep: false, compositionStatus: '' },
      { id: 2, name: 'Jarabe natural', onHand: '0', baseUnitCode: 'ml', baseUnitKind: 'volumen', baseUnitId: 3, isPrep: true, compositionStatus: 'estimated', countPlaceId: 4, countPlace: 'Barra' },
    ],
  })),
  articles: vi.fn(() => Promise.resolve([
    { id: 1, name: 'Azúcar', isActive: true, baseUnitId: 1, baseUnitCode: 'g', baseUnitKind: 'masa', currentCost: '0', minStock: null, isPrep: false, categoryId: null, category: null, countPlaceId: null, countPlace: null, onHand: '1200', presentations: [] },
    { id: 2, name: 'Jarabe natural', isActive: true, baseUnitId: 3, baseUnitCode: 'ml', baseUnitKind: 'volumen', currentCost: '0.05', minStock: null, isPrep: true, categoryId: null, category: null, countPlaceId: 4, countPlace: 'Barra', onHand: '0', presentations: [] },
  ])),
  article: vi.fn(() => Promise.resolve({ id: 2, name: 'Jarabe natural', isActive: true, baseUnitId: 3, baseUnitCode: 'ml', baseUnitKind: 'volumen', currentCost: '0.05', minStock: null, isPrep: true, categoryId: null, category: null, countPlaceId: 4, countPlace: 'Barra', onHand: '0', presentations: [], costSource: 'receta', updatedAt: '2026-10-19T10:00:00Z' })),
  articleCostHistory: vi.fn(() => Promise.resolve({ items: [], next: null, trendPct: null })),
  articleCategories: vi.fn(() => Promise.resolve([])),
  currentCount: vi.fn(() => Promise.resolve({ id: 5, openedAt: '2026-10-14T13:40:00Z', items: [
    { ingredientId: 2, name: 'Jarabe natural', place: 'Barra', placeId: 4, baseUnitCode: 'ml', presentations: [], countedQty: null },
    { ingredientId: 1, name: 'Azúcar', place: null, placeId: null, baseUnitCode: 'g', presentations: [], countedQty: null },
  ] })),
  inventoryHealth: vi.fn(() => Promise.resolve({
    from: '2026-09-14', to: '2026-10-13', multipleBranches: false, salesInPeriod: 0,
    unsoldDeduction: { total: 0, items: [], unpairedPlatformLines: 0 }, comboChoices: { total: 0, items: [] },
    negativeStock: { total: 0, collapsed: false, items: [] }, noPurchases: { total: 0, of: 0, items: [] },
    malformedRecipes: { total: 0, items: [] },
    pendingRecipes: { undecided: { products: 0, extras: 0, fixAt: '' }, review: { products: 0, extras: 0, fixAt: '' } },
  })),
}));
const admin = vi.hoisted(() => ({
  composition: vi.fn(() => Promise.resolve({ status: 'estimated', items: [], components: [], editable: true, yield: '1000', yieldUnitCode: 'ml', stamp: '', sameName: [] })),
  recipes: vi.fn(() => Promise.resolve({ items: [], total: 0, counts: { pending: 0, review: 0, done: 0 }, totals: {} })),
  products: vi.fn(() => Promise.resolve({ items: [], total: 0, counts: { act: 0, inact: 0 } })),
}));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
// Cada prueba arranca con un solo almacén y sin existencias: una que cambie el almacén y falle a la
// mitad no deja a las siguientes con dos.
beforeEach(() => {
  back.warehouses.mockResolvedValue(oneWarehouse);
  back.stockLevels.mockImplementation(() => Promise.resolve({ items: [] }));
});
vi.mock('../../api/admin', () => ({ adminApi: admin }));
const pos = vi.hoisted(() => ({ businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' } as Record<string, string>)) }));
vi.mock('../../api/pos', () => ({ posApi: pos }));
const notif = vi.hoisted(() => ({
  recipients: { get: vi.fn(() => Promise.resolve({ morningAt: '08:00', extras: [], people: [] })), save: vi.fn() },
  shopping: { list: vi.fn(() => Promise.resolve({ items: [] })), email: vi.fn() },
}));
vi.mock('../../api/notifications', async (orig) => ({ ...(await orig<object>()), recipientsApi: notif.recipients, shoppingApi: notif.shopping }));

function montar(entrada = '/almacen') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><MemoryRouter initialEntries={[entrada]}>
    <Routes><Route path="/almacen" element={<StockPage />} /><Route path="/negocio" element={<Donde />} /></Routes>
  </MemoryRouter></Provider></QueryClientProvider>);
}

function Donde() {
  const l = useLocation();
  return <p>{l.pathname + l.search + l.hash}</p>;
}

function sesion(permissions: string[]) {
  // El rol es «cajero» a propósito: la pestaña la decide el permiso, nunca el nombre del rol.
  useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'cajero', permissions } as never });
}

// ARTÍCULOS (spec 042): la pestaña es la lista con buscador y filtros; cada renglón abre su ficha, y
// desde la ficha se llega a la receta. Lo que antes vivía en columnas (empaques, lugar, receta) está
// en la ficha.
describe('Almacén › Artículos', () => {
  beforeEach(() => vi.clearAllMocks());

  it('tocar un artículo abre su ficha, y desde ahí su receta', async () => {
    sesion(['inventory.view', 'inventory.configure']);
    montar('/almacen?tab=insumos');
    fireEvent.click(await screen.findByRole('button', { name: /Jarabe natural/ }));
    expect(await screen.findByRole('button', { name: 'Bitácora' })).toBeInTheDocument();
    expect(back.article).toHaveBeenCalledWith(2);
    fireEvent.click(await screen.findByRole('button', { name: 'Ver receta ›' }));
    expect(await screen.findByText('Para prepararlo se usa:')).toBeInTheDocument();
    expect(admin.composition).toHaveBeenCalledWith('ingredient', 2);
  });
});

// ALMACÉN › EXISTENCIAS (spec 077): la lista ya no trae insumos archivados, así que puede quedar vacía
// aunque haya movimientos (el último insumo con existencia se archivó). «Sin movimientos aún» mentía.
describe('Almacén › Existencias', () => {
  beforeEach(() => vi.clearAllMocks());

  it('vacía, dice que no hay existencias que mostrar y no que no hay movimientos', async () => {
    sesion(['inventory.view']);
    back.stockMovements.mockResolvedValueOnce({ items: [{ id: 1, item_type: 'ingrediente', item_name: 'Jarabe', movement_type: 'ajuste', quantity: '-3', reason: null, created_at: '2026-11-12T10:00:00Z' }] } as never);
    montar('/almacen');
    expect(await screen.findByText('Sin existencias que mostrar')).toBeInTheDocument();
    expect(screen.queryByText('Sin movimientos aún')).not.toBeInTheDocument();
  });
});

// ALMACÉN › SALUD (spec 037): la pestaña sale por permiso y Almacén deja de crecer con la página.
describe('Almacén › pestañas', () => {
  beforeEach(() => vi.clearAllMocks());

  it('la pestaña Salud aparece con el permiso y no sin él, sin mirar el rol', async () => {
    sesion(['inventory.health_view']);
    const { unmount } = montar();
    expect(await screen.findByRole('tab', { name: 'Salud' })).toBeInTheDocument();
    unmount();
    sesion([]);
    montar();
    await screen.findByRole('tab', { name: 'Existencias' });
    expect(screen.queryByRole('tab', { name: 'Salud' })).not.toBeInTheDocument();
  });

  it('las pestañas miden al menos 44 px y las de antes siguen funcionando', async () => {
    sesion(['inventory.health_view', 'inventory.view']);
    montar();
    for (const name of ['Existencias', 'Movimientos', 'Insumos', 'Salud']) {
      const tab = await screen.findByRole('tab', { name });
      expect(Number.parseInt(getComputedStyle(tab).minHeight, 10)).toBeGreaterThanOrEqual(44);
    }
    fireEvent.click(screen.getByRole('tab', { name: 'Insumos' }));
    expect(await screen.findByText('Jarabe natural')).toBeInTheDocument();
  });

  it('la dirección abre la pestaña y la ficha que pide Salud', async () => {
    sesion(['inventory.health_view', 'inventory.view']);
    montar('/almacen?tab=insumos&open=2');
    expect(await screen.findByRole('button', { name: 'Bitácora' })).toBeInTheDocument();
    expect(back.article).toHaveBeenCalledWith(2);
  });
});

// Capturas del 038: la pestaña ya decía «Artículos» con la palabra de la empresa y la columna seguía
// diciendo «Insumo».
it('la columna usa la misma palabra que la pestaña', async () => {
  pos.businessSettings.mockResolvedValueOnce({ timezone: 'America/Mexico_City', stockItemLabel: 'artículo' });
  montar('/almacen?tab=insumos');
  expect(await screen.findByRole('tab', { name: 'Artículos' })).toBeInTheDocument();
  expect(await screen.findByRole('columnheader', { name: 'Artículo' })).toBeInTheDocument();
});

it('la lista usa la palabra de la empresa en la columna y en el alta', async () => {
  sesion(['inventory.view', 'inventory.configure']);
  pos.businessSettings.mockResolvedValueOnce({ timezone: 'America/Mexico_City', stockItemLabel: 'insumo' });
  montar('/almacen?tab=insumos');
  expect(await screen.findByRole('columnheader', { name: 'Insumo' })).toBeInTheDocument();
  expect(screen.getByLabelText('Buscar insumo o empaque')).toBeInTheDocument();
});

// Existencias marca un negativo igual que Artículos: la cifra en rojo y la etiqueta «Revisar». El
// renglón entero en rojo era otro diseño para lo mismo (decisión del dueño, 2026-10-21).
it('Existencias marca un negativo con «Revisar» y sin pintar el renglón entero', async () => {
  back.stockLevels.mockResolvedValueOnce({ items: [
    { item_name: 'Leche', item_type: 'insumo', on_hand: '-3000', unit_code: 'ml', min_stock: null },
    { item_name: 'Azúcar', item_type: 'insumo', on_hand: '1200', unit_code: 'g', min_stock: null },
  ] } as never);
  sesion(['inventory.configure']);
  montar('/almacen');
  const fila = (await screen.findByText('Leche')).closest('tr')!;
  expect(fila.textContent).toContain('Revisar');
  expect(getComputedStyle(fila).backgroundColor).toBe(getComputedStyle((await screen.findByText('Azúcar')).closest('tr')!).backgroundColor);
  expect((await screen.findByText('Azúcar')).closest('tr')!.textContent).not.toContain('Revisar');
});

// Spec 045: Existencias decía «-3000 ml» y Artículos «-3 L» para el mismo artículo; las dos pantallas
// dicen la cifra con la misma regla, también el mínimo y el plural de pieza.
it('Existencias dice la cantidad igual que Artículos: -3 L, 1 pieza', async () => {
  back.stockLevels.mockResolvedValueOnce({ items: [
    { item_name: 'Leche', item_type: 'insumo', on_hand: '-3000', unit_code: 'ml', min_stock: '2000' },
    { item_name: 'Vaso', item_type: 'insumo', on_hand: '1', unit_code: 'pieza', min_stock: null },
  ] } as never);
  sesion(['inventory.configure']);
  montar('/almacen');
  const leche = (await screen.findByText('Leche')).closest('tr')!;
  expect(leche.textContent).toContain('-3 L');
  expect(leche.textContent).not.toContain('-3000');
  expect(leche.textContent).toContain('2 L');
  expect((await screen.findByText('Vaso')).closest('tr')!.textContent).toContain('1 pieza');
});

// Spec 045 (US2): desde la ficha de un artículo en negativo, un toque abre el conteo ya buscado.
it('«Revisar con un conteo» cierra la ficha y abre la pestaña Conteo con el artículo buscado', async () => {
  back.article.mockResolvedValueOnce({ id: 2, name: 'Jarabe natural', isActive: true, baseUnitId: 3, baseUnitCode: 'ml', baseUnitKind: 'volumen', currentCost: '0.05', minStock: null, isPrep: true, categoryId: null, category: null, countPlaceId: 4, countPlace: 'Barra', onHand: '-3000', presentations: [], costSource: 'receta', updatedAt: '2026-10-19T10:00:00Z' } as never);
  sesion(['inventory.view', 'inventory.count']);
  montar('/almacen?tab=insumos&open=2');
  fireEvent.click(await screen.findByRole('button', { name: /Revisar con un conteo/ }));
  expect(await screen.findByLabelText('Buscar')).toHaveValue('Jarabe natural');
  expect(screen.getByRole('tab', { name: 'Conteo' })).toHaveAttribute('aria-selected', 'true');
  expect(screen.queryByRole('button', { name: /Revisar con un conteo/ })).toBeNull();
});

// Lo que se edita en Avisos no se pierde al ir a otra pestaña y volver: antes la pestaña se
// desmontaba y el correo a medio capturar desaparecía sin aviso (revisión de la 084).
describe('Almacén › Avisos', () => {
  beforeEach(() => vi.clearAllMocks());

  it('un correo a medio capturar sigue ahí al volver de otra pestaña', async () => {
    sesion(['inventory.view', 'inventory.configure']);
    montar('/almacen?tab=avisos');
    fireEvent.change(await screen.findByLabelText('Correo extra'), { target: { value: 'compras@ejemplo.mx' } });
    await userEvent.click(screen.getByRole('tab', { name: 'Lista de compra' }));
    expect(await screen.findByRole('tab', { name: 'Lista de compra', selected: true })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('tab', { name: 'Avisos' }));
    expect(((await screen.findByLabelText('Correo extra')) as HTMLInputElement).value).toBe('compras@ejemplo.mx');
  });
});

// Existencias por almacén (spec 094, maqueta A): con un solo almacén la pantalla queda como hoy; con
// dos aparece el segmentado, por omisión el de venta, y «Todos» suma.
it('con un solo almacén no hay selector', async () => {
  sesion(['inventory.view']);
  montar('/almacen');
  await screen.findByRole('columnheader', { name: 'Artículo' });
  expect(screen.queryByRole('group', { name: 'Almacén' })).toBeNull();
  expect(back.stockLevels).toHaveBeenLastCalledWith(undefined);
});

it('con dos almacenes elige cuál ver, y «Todos» suma', async () => {
  back.warehouses.mockResolvedValue({ items: [
    { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 2 },
    { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 1 },
  ] });
  sesion(['inventory.view']);
  montar('/almacen');
  const grupo = await screen.findByRole('group', { name: 'Almacén' });
  expect(screen.getByRole('button', { name: 'Local · de venta' })).toHaveAttribute('aria-pressed', 'true');
  fireEvent.click(screen.getByRole('button', { name: 'Bodega' }));
  await vi.waitFor(() => expect(back.stockLevels).toHaveBeenLastCalledWith(2));
  fireEvent.click(screen.getByRole('button', { name: 'Todos' }));
  await vi.waitFor(() => expect(back.stockLevels).toHaveBeenLastCalledWith('all'));
  expect(grupo).toBeInTheDocument();
});

// Cada almacén se compara con SU mínimo, el que manda el servidor: la Bodega con 10 L de mínimo decía
// «—» y nunca «bajo» (prueba con alguien nuevo, 094). En «Todos» el servidor no manda mínimo.
it('cada almacén muestra y compara su propio mínimo', async () => {
  back.warehouses.mockResolvedValue({ items: [
    { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 1 },
    { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 1 },
  ] });
  back.stockLevels.mockImplementation((w?: number | 'all') => Promise.resolve({ items: [
    { item_name: 'Leche', item_type: 'insumo', on_hand: w === 2 ? '8000' : '1000', unit_code: 'ml',
      min_stock: w === 2 ? '10000' : w === 'all' ? null : '5000', is_active: true },
  ] }));
  sesion(['inventory.view']);
  montar('/almacen');
  expect(((await screen.findByText('Leche')).closest('tr')!).textContent).toContain('bajo');
  fireEvent.click(await screen.findByRole('button', { name: 'Bodega' }));
  await vi.waitFor(() => expect(screen.getByText('Leche').closest('tr')!.textContent).toContain('8 L'));
  expect(screen.getByText('Leche').closest('tr')!.textContent).toContain('10 L');
  expect(screen.getByText('Leche').closest('tr')!.textContent).toContain('bajo');
  fireEvent.click(screen.getByRole('button', { name: 'Todos' }));
  await vi.waitFor(() => expect(back.stockLevels).toHaveBeenLastCalledWith('all'));
  await vi.waitFor(() => expect(screen.getByText('Leche').closest('tr')!.textContent).not.toContain('bajo'));
});

// Un error al pedir otro almacén no se pinta como «sin existencias».
it('si no se pudo leer el almacén lo dice', async () => {
  back.warehouses.mockResolvedValue({ items: [
    { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 1 },
    { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 1 },
  ] });
  back.stockLevels.mockImplementation((w?: number | 'all') => (w === 2 ? Promise.reject(new Error('x')) : Promise.resolve({ items: [] })));
  sesion(['inventory.view']);
  montar('/almacen');
  fireEvent.click(await screen.findByRole('button', { name: 'Bodega' }));
  expect(await screen.findByText('No se pudieron leer las existencias de este almacén')).toBeInTheDocument();
  expect(screen.queryByText('Sin existencias que mostrar')).toBeNull();
});

// La persona nueva buscó los almacenes en Almacén y no estaban (prueba con usuario nuevo, 094): quien
// los administra tiene el atajo junto al título.
it('quien administra almacenes llega a ellos desde Almacén', async () => {
  sesion(['inventory.view', 'inventory.warehouse_manage']);
  montar('/almacen');
  fireEvent.click(await screen.findByRole('button', { name: 'Almacenes ›' }));
  // Abre la sección de Inventario de Negocio, no la primera (prueba con usuario nuevo, 094).
  expect(await screen.findByText('/negocio?seccion=inventario#almacenes')).toBeInTheDocument();
});

it('quien no los administra no ve el atajo', async () => {
  sesion(['inventory.view']);
  montar('/almacen');
  await screen.findByRole('columnheader', { name: 'Artículo' });
  expect(screen.queryByRole('button', { name: 'Almacenes ›' })).toBeNull();
});

it('una bodega vacía lo dice', async () => {
  back.warehouses.mockResolvedValue({ items: [
    { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 1 },
    { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 0 },
  ] });
  sesion(['inventory.view']);
  montar('/almacen');
  fireEvent.click(await screen.findByRole('button', { name: 'Bodega' }));
  expect(await screen.findByText('Este almacén todavía no tiene mercancía')).toBeInTheDocument();
});

// TRASPASOS (spec 094, subida 3).
const twoWarehouses = { items: [
  { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 1 },
  { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 1 },
] };

it('la pestaña Traspaso sale con el permiso y dos almacenes, después de Existencias', async () => {
  back.warehouses.mockResolvedValue(twoWarehouses);
  sesion(['inventory.view', 'inventory.transfer']);
  montar('/almacen');
  const tabs = await screen.findAllByRole('tab');
  await vi.waitFor(() => expect(screen.getAllByRole('tab').map((t) => t.textContent)).toContain('Traspaso'));
  const names = screen.getAllByRole('tab').map((t) => t.textContent);
  expect(names.indexOf('Traspaso')).toBe(names.indexOf('Existencias') + 1);
  expect(tabs.length).toBeGreaterThan(0);
});

it('sin el permiso o con un solo almacén no hay pestaña Traspaso', async () => {
  back.warehouses.mockResolvedValue(twoWarehouses);
  sesion(['inventory.view']);
  montar('/almacen');
  await screen.findByRole('columnheader', { name: 'Artículo' });
  expect(screen.queryByRole('tab', { name: 'Traspaso' })).toBeNull();
  cleanup();
  back.warehouses.mockResolvedValue(oneWarehouse);
  sesion(['inventory.view', 'inventory.transfer']);
  montar('/almacen');
  await screen.findByRole('columnheader', { name: 'Artículo' });
  expect(screen.queryByRole('tab', { name: 'Traspaso' })).toBeNull();
});

it('un negativo en el de venta con mercancía en la bodega se trae con un toque', async () => {
  back.warehouses.mockResolvedValue(twoWarehouses);
  back.stockLevels.mockImplementation(() => Promise.resolve({ items: [
    { item_name: 'Leche', item_type: 'ingrediente', ingredient_id: 7, product_id: null, on_hand: '-2000', unit_code: 'ml', min_stock: null, is_active: true,
      transfer_from: [{ warehouseId: 2, name: 'Bodega', onHand: '20000' }] },
    { item_name: 'Azúcar', item_type: 'ingrediente', ingredient_id: 8, product_id: null, on_hand: '-500', unit_code: 'g', min_stock: null, is_active: true },
  ] } as never));
  sesion(['inventory.view', 'inventory.transfer']);
  montar('/almacen');
  const fila = (await screen.findByText('Leche')).closest('tr')!;
  fireEvent.click(within(fila).getByRole('button', { name: 'Traer de Bodega' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Traspasar 2 L' }));
  await vi.waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(1));
  expect(back.createTransfer.mock.calls[0][0]).toMatchObject({ fromWarehouseId: 2, toWarehouseId: 1,
    lines: [{ itemType: 'ingrediente', ingredientId: 7, qty: '2000' }] });
  // Sin de dónde traer, no hay botón.
  expect(within((await screen.findByText('Azúcar')).closest('tr')!).queryByRole('button', { name: /Traer de/ })).toBeNull();
});

// MOVIMIENTOS Y VALOR POR ALMACÉN (spec 094, subida 5).
const moveLeche = (over: Record<string, unknown>) => ({ id: 1, item_type: 'insumo', item_name: 'Leche', movement_type: 'venta',
  quantity: '-6000', reason: null, created_at: '2026-11-20T15:00:00Z', unit_code: 'ml', warehouse_id: 1, warehouse_name: 'Local', ...over });

it('con un solo almacén, Movimientos queda como antes: sin selector ni columna de almacén', async () => {
  back.stockMovements.mockImplementation(() => Promise.resolve({ items: [moveLeche({})] } as never));
  sesion(['inventory.view']);
  montar('/almacen?tab=movimientos');
  await screen.findByText('Leche');
  expect(screen.queryByRole('group', { name: 'Almacén' })).toBeNull();
  expect(screen.queryByRole('columnheader', { name: 'Almacén' })).toBeNull();
  expect(back.stockMovements).toHaveBeenLastCalledWith(undefined);
});

it('Movimientos dice la cantidad con su unidad, no «-6000»', async () => {
  back.stockMovements.mockImplementation(() => Promise.resolve({ items: [moveLeche({})] } as never));
  sesion(['inventory.view']);
  montar('/almacen?tab=movimientos');
  const fila = (await screen.findByText('Leche')).closest('tr')!;
  expect(fila.textContent).toContain('-6 L');
  expect(fila.textContent).not.toContain('-6000');
});

it('con dos almacenes, Movimientos elige almacén y solo «Todos» trae la columna Almacén', async () => {
  back.warehouses.mockResolvedValue(twoWarehouses);
  back.stockMovements.mockImplementation(() => Promise.resolve({ items: [moveLeche({})] } as never));
  sesion(['inventory.view']);
  montar('/almacen?tab=movimientos');
  await screen.findByText('Leche');
  expect(await screen.findByRole('group', { name: 'Almacén' })).toBeInTheDocument();
  expect(screen.queryByRole('columnheader', { name: 'Almacén' })).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Bodega' }));
  await vi.waitFor(() => expect(back.stockMovements).toHaveBeenLastCalledWith(2));
  fireEvent.click(screen.getByRole('button', { name: 'Todos' }));
  await vi.waitFor(() => expect(back.stockMovements).toHaveBeenLastCalledWith('all'));
  expect(await screen.findByRole('columnheader', { name: 'Almacén' })).toBeInTheDocument();
  await vi.waitFor(() => expect(screen.getByText('Leche').closest('tr')!.textContent).toContain('Local'));
});

it('un traspaso dice a dónde fue o de dónde vino', async () => {
  back.warehouses.mockResolvedValue(twoWarehouses);
  back.stockMovements.mockImplementation(() => Promise.resolve({ items: [
    moveLeche({ id: 1, movement_type: 'traspaso', quantity: '-2000', warehouse_id: 2, warehouse_name: 'Bodega', transfer_id: 9, counterpart_warehouse_name: 'Local', item_name: 'Leche sale' }),
    moveLeche({ id: 2, movement_type: 'traspaso', quantity: '2000', transfer_id: 9, counterpart_warehouse_name: 'Bodega', item_name: 'Leche entra' }),
  ] } as never));
  sesion(['inventory.view']);
  montar('/almacen?tab=movimientos');
  expect((await screen.findByText('Leche sale')).closest('tr')!.textContent).toContain('→ Local');
  expect(screen.getByText('Leche entra').closest('tr')!.textContent).toContain('← Bodega');
  // Prueba con alguien nuevo: a 1024×600 «traspaso → Bodega» se partía en dos renglones (fila de 57 px).
  const tipo = within(screen.getByText('Leche sale').closest('tr')!).getByText(/traspaso/);
  expect(getComputedStyle(tipo).whiteSpace).toBe('nowrap');
});

// El total va arriba, junto al selector: al pie de 440 renglones nadie lo encontraba (prueba con
// alguien nuevo, 094). En «Todos» se desglosa por almacén sin cambiar de filtro.
it('Existencias trae el valor de cada artículo y el total arriba', async () => {
  back.stockLevels.mockImplementation(() => Promise.resolve({ totalValue: '1250.5', items: [
    { item_name: 'Leche', item_type: 'insumo', on_hand: '10000', unit_code: 'ml', min_stock: null, is_active: true, value: '250.5' },
    { item_name: 'Café', item_type: 'insumo', on_hand: '-100', unit_code: 'g', min_stock: null, is_active: true, value: '0' },
  ] } as never));
  sesion(['inventory.view']);
  montar('/almacen');
  expect(await screen.findByRole('columnheader', { name: 'Valor' })).toBeInTheDocument();
  expect((await screen.findByText('Leche')).closest('tr')!.textContent).toContain('$250.50');
  // El total es el que manda el servidor (no suma negativos), no una suma hecha aquí, y no vive en la tabla.
  const total = screen.getByText('Valor total');
  expect(total.closest('table')).toBeNull();
  expect(screen.getByTestId('valor-total').textContent).toContain('$1,250.50');
  fireEvent.click(screen.getByRole('button', { name: '¿Qué suma el valor total?' }));
  expect(await screen.findByText(/no resta/i)).toBeInTheDocument();
});

it('en «Todos» el valor se desglosa por almacén', async () => {
  back.warehouses.mockResolvedValue({ items: [
    { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 1 },
    { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 1 },
  ] });
  back.stockLevels.mockImplementation((w?: number | 'all') => Promise.resolve(w === 'all'
    ? { totalValue: '1300', valueByWarehouse: [{ warehouseId: 1, warehouseName: 'Local', value: '1000' }, { warehouseId: 2, warehouseName: 'Bodega', value: '300' }],
      items: [{ item_name: 'Leche', item_type: 'insumo', on_hand: '1000', unit_code: 'ml', min_stock: null, is_active: true, value: '1300' }] }
    : { totalValue: '1000', items: [{ item_name: 'Leche', item_type: 'insumo', on_hand: '1000', unit_code: 'ml', min_stock: null, is_active: true, value: '1000' }] }) as never);
  sesion(['inventory.view']);
  montar('/almacen');
  fireEvent.click(await screen.findByRole('button', { name: 'Todos' }));
  await vi.waitFor(() => expect(screen.getByTestId('valor-total').textContent).toContain('$1,300'));
  expect(screen.getByTestId('valor-total').textContent).toMatch(/Local \$1,000.*Bodega \$300/);
});

// Movimientos: la fecha no se parte en renglones, el motivo largo se recorta con el texto completo al
// tocarlo, y un traspaso con un almacén archivado lo dice (había dos «Bodega» y no se distinguían).
it('Movimientos dice cuándo la contraparte está archivada y no parte la fecha', async () => {
  back.stockMovements.mockImplementation(() => Promise.resolve({ items: [
    moveLeche({ id: 2, movement_type: 'traspaso', quantity: '2000', transfer_id: 9, counterpart_warehouse_name: 'Bodega',
      counterpart_warehouse_archived: true, item_name: 'Leche entra', reason: 'Un motivo muy largo que escribió alguien para explicar todo lo que pasó con la leche' }),
  ] } as never));
  sesion(['inventory.view']);
  montar('/almacen?tab=movimientos');
  const fila = (await screen.findByText('Leche entra')).closest('tr')!;
  expect(fila.textContent).toContain('← Bodega (archivado)');
  const celdas = fila.querySelectorAll('td');
  expect(getComputedStyle(celdas[0]).whiteSpace).toBe('nowrap');
  // En tableta no hay title que se vea: el motivo se lee en dos renglones antes de recortarse.
  const motivo = within(fila).getByText(/Un motivo muy largo/);
  expect(getComputedStyle(motivo).webkitLineClamp ?? motivo.style.getPropertyValue('-webkit-line-clamp')).toBe('2');
});

it('sin valor del servidor no hay columna Valor ni total', async () => {
  back.stockLevels.mockImplementation(() => Promise.resolve({ items: [
    { item_name: 'Leche', item_type: 'insumo', on_hand: '10000', unit_code: 'ml', min_stock: null, is_active: true },
  ] } as never));
  sesion(['inventory.view']);
  montar('/almacen');
  await screen.findByText('Leche');
  expect(screen.queryByRole('columnheader', { name: 'Valor' })).toBeNull();
  expect(screen.queryByText('Valor total')).toBeNull();
});

// Spec 098: Caducidades con el permiso de ver inventario; el aviso de Existencias solo cuando hay algo
// y lleva a la pestaña.
it('el aviso de caducidad sale con algo por caducar y lleva a Caducidades', async () => {
  back.warehouses.mockResolvedValue(oneWarehouse);
  back.expirySummary.mockResolvedValue({ expired: 1, within7: 2 });
  sesion(['inventory.view']);
  montar('/almacen');
  const aviso = await screen.findByTestId('aviso-caducidad');
  expect(aviso).toHaveTextContent('1 caducado · 2 caducan en 7 días');
  fireEvent.click(screen.getByRole('button', { name: 'Ver caducidades ›' }));
  expect(await screen.findByTestId('sin-caducidades')).toBeInTheDocument();
});

it('sin nada por caducar no hay aviso', async () => {
  back.warehouses.mockResolvedValue(oneWarehouse);
  back.expirySummary.mockResolvedValue({ expired: 0, within7: 0 });
  sesion(['inventory.view']);
  montar('/almacen');
  await screen.findByRole('columnheader', { name: 'Artículo' });
  expect(screen.queryByTestId('aviso-caducidad')).toBeNull();
  expect(screen.getByRole('tab', { name: 'Caducidades' })).toBeInTheDocument();
});
