import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';
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

// LOS INSUMOS QUE SE PREPARAN EN EL LOCAL SE VEN Y SE CORRIGEN AQUÍ (spec 028).
//
// Hasta ahora un insumo preparado —el que cargaba FUDO como estimado— no tenía dónde verse: si venía
// mal, descontaba mal para siempre.
describe('Almacén › Insumos', () => {
  beforeEach(() => vi.clearAllMocks());

  it('dice cuáles se preparan aquí y cuáles faltan por revisar', async () => {
    montar();
    fireEvent.click(await screen.findByRole('tab', { name: 'Insumos' }));
    const jarabe = (await screen.findByText('Jarabe natural')).closest('tr')!;
    expect(jarabe).toHaveTextContent('Es un preparado');
    expect(jarabe).toHaveTextContent('por revisar');
    expect((await screen.findByText('Azúcar')).closest('tr')!).toHaveTextContent('Se compra hecho');
  });

  it('tocar el insumo abre su receta', async () => {
    montar();
    fireEvent.click(await screen.findByRole('tab', { name: 'Insumos' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Receta de Jarabe natural' }));
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
    sesion(['inventory.health_view']);
    montar();
    for (const name of ['Existencias', 'Movimientos', 'Insumos', 'Salud']) {
      const tab = await screen.findByRole('tab', { name });
      expect(Number.parseInt(getComputedStyle(tab).minHeight, 10)).toBeGreaterThanOrEqual(44);
    }
    fireEvent.click(screen.getByRole('tab', { name: 'Insumos' }));
    expect(await screen.findByText('Jarabe natural')).toBeInTheDocument();
  });

  it('la dirección abre la pestaña y el insumo que pide Salud', async () => {
    sesion(['inventory.health_view']);
    montar('/almacen?tab=insumos&open=2');
    expect(await screen.findByText('Para prepararlo se usa:')).toBeInTheDocument();
    expect(admin.composition).toHaveBeenCalledWith('ingredient', 2);
  });

  // Cómo llega cada insumo (spec 038, maqueta «Cómo llega un insumo», opción B): a la vista en la
  // lista, para ver de un vistazo cuál no tiene empaque.
  it('con el permiso de compras, cada insumo dice cómo llega y se puede cambiar', async () => {
    sesion(['inventory.receive', 'inventory.configure', 'inventory.count']);
    montar('/almacen?tab=insumos');
    const jarabe = (await screen.findByText('Jarabe natural')).closest('tr')!;
    expect(await screen.findByText('Botella · 750 ml')).toBeInTheDocument();
    expect(jarabe).toHaveTextContent('Ya no llega en: Garrafa');
    const azucar = screen.getByText('Azúcar').closest('tr')!;
    expect(azucar).toHaveTextContent('Sin empaques');
    // La prueba sin contexto (040, T022) no encontró dónde está el lugar, y «+ Empaque o lugar» juntaba
    // dos cosas: el lugar tiene su propio botón en el renglón y el de empaques solo habla de empaques.
    expect(await within(jarabe).findByRole('button', { name: /Barra/ })).toBeInTheDocument();
    expect(within(azucar).getByRole('button', { name: /Sin lugar/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cómo llega Jarabe natural' })).toHaveTextContent('Empaques ›');
    fireEvent.click(screen.getByRole('button', { name: 'Cómo llega Jarabe natural' }));
    expect(await screen.findByRole('dialog')).toHaveTextContent('Cómo llega Jarabe natural');
  });

  it('sin el permiso de compras no se pide ni se muestra la columna', async () => {
    sesion([]);
    montar('/almacen?tab=insumos');
    await screen.findByText('Jarabe natural');
    expect(screen.queryByText('Cómo llega')).toBeNull();
    expect(back.presentations).not.toHaveBeenCalled();
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
