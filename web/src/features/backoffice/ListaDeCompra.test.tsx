import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ListaDeCompra } from './ListaDeCompra';

const api = vi.hoisted(() => ({ list: vi.fn(), emailMe: vi.fn() }));
vi.mock('../../api/notifications', async (orig) => ({ ...(await orig<object>()), shoppingApi: api }));

beforeEach(() => {
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
