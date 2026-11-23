import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ListaDeCompra } from './ListaDeCompra';

const api = vi.hoisted(() => ({ list: vi.fn(), emailMe: vi.fn() }));
vi.mock('../../api/notifications', async (orig) => ({ ...(await orig<object>()), shoppingApi: api }));
const back = vi.hoisted(() => ({ warehouses: vi.fn() }));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
import { useSessionStore } from '../../stores/session';

beforeEach(() => {
  back.warehouses.mockResolvedValue({ items: [{ id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 2 }] });
  useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'gerente', permissions: ['inventory.view'] } as never });
  api.list.mockResolvedValue({ items: [
    { itemType: 'ingrediente', id: 1, name: 'Bolsa grande', unit: 'pieza', onHand: '0', min: '50', dailyUse: '22', coverageDays: 14,
      supplier: 'Desechables del Centro', state: 'out', suggested: { packs: 4, packName: 'Paquete de 100', qty: '400' }, why: 'se usan ~22 pzas al día, quedan 0 pzas, objetivo 14 días' },
    { itemType: 'ingrediente', id: 2, name: 'Jarabe de taro', unit: 'ml', onHand: '800', min: '1500', dailyUse: null, coverageDays: 14,
      supplier: '', state: 'low', suggested: null, why: 'sin consumo medido: no está en ninguna receta o casi no se ha vendido' },
  ] });
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

describe('Lista de compra', () => {
  test('agrupa por lugar de compra y explica cada cantidad', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={qc}><Provider><ListaDeCompra /></Provider></QueryClientProvider>);
    expect(await screen.findByText('Desechables del Centro')).toBeTruthy();
    expect(screen.getByText('Sin lugar de compra')).toBeTruthy();
    expect(screen.getByText('4 × Paquete de 100')).toBeTruthy();
    expect(screen.getByText(/se usan ~22 pzas al día/)).toBeTruthy();
    expect(screen.getByText('Sin sugerencia')).toBeTruthy();
    expect(screen.getByText(/sin consumo medido/)).toBeTruthy();
  });
});

// VARIOS ALMACENES (spec 094): con dos o más, cada renglón dice de qué almacén es lo que hay.
describe('lista de compra por almacén (spec 094)', () => {
  const pintar = () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={qc}><Provider><ListaDeCompra /></Provider></QueryClientProvider>);
  };
  const conAlmacen = () => api.list.mockResolvedValue({ items: [
    { itemType: 'ingrediente', id: 2, name: 'Jarabe de taro', unit: 'ml', onHand: '800', min: '1500', dailyUse: null, coverageDays: 14,
      supplier: '', state: 'low', suggested: null, why: 'x', warehouseId: 1, warehouseName: 'Local' },
  ] });
  test('con un solo almacén el renglón queda como antes', async () => {
    conAlmacen();
    pintar();
    expect((await screen.findByText('Jarabe de taro')).parentElement!.textContent).not.toContain('Local');
  });
  test('con dos dice de qué almacén', async () => {
    back.warehouses.mockResolvedValue({ items: [
      { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 2 },
      { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 1 },
    ] });
    conAlmacen();
    pintar();
    await screen.findByText('Jarabe de taro');
    expect(await screen.findByText(/Hay 800 ml en Local/)).toBeTruthy();
  });
});
