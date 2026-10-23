import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';

import { useSessionStore } from '../../stores/session';

import { Provider } from '../../components/ui/provider';
import { StockPage } from './StockPage';

const back = vi.hoisted(() => ({
  countPlaces: vi.fn(() => Promise.resolve([{ id: 4, name: 'Barra', position: 0, ingredients: 1 }])),
  stockLevels: vi.fn(() => Promise.resolve({ items: [] })),
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
vi.mock('../../api/admin', () => ({ adminApi: admin }));
const pos = vi.hoisted(() => ({ businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' } as Record<string, string>)) }));
vi.mock('../../api/pos', () => ({ posApi: pos }));

function montar(entrada = '/almacen') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><MemoryRouter initialEntries={[entrada]}><StockPage /></MemoryRouter></Provider></QueryClientProvider>);
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
    fireEvent.click(await screen.findByRole('button', { name: /Es un preparado · ver receta/ }));
    expect(await screen.findByText('Para prepararlo se usa:')).toBeInTheDocument();
    expect(admin.composition).toHaveBeenCalledWith('ingredient', 2);
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
