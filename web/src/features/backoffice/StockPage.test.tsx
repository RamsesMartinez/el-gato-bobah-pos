import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { StockPage } from './StockPage';

const back = vi.hoisted(() => ({
  stockLevels: vi.fn(() => Promise.resolve({ items: [] })),
  stockMovements: vi.fn(() => Promise.resolve({ items: [] })),
  units: vi.fn(() => Promise.resolve({ items: [] })),
  ingredients: vi.fn(() => Promise.resolve({
    items: [
      { id: 1, name: 'Azúcar', onHand: '1200', baseUnitCode: 'g', baseUnitKind: 'masa', baseUnitId: 1, isPrep: false, compositionStatus: '' },
      { id: 2, name: 'Jarabe natural', onHand: '0', baseUnitCode: 'ml', baseUnitKind: 'volumen', baseUnitId: 3, isPrep: true, compositionStatus: 'estimated' },
    ],
  })),
}));
const admin = vi.hoisted(() => ({
  composition: vi.fn(() => Promise.resolve({ status: 'estimated', items: [], components: [], editable: true, yield: '1000', yieldUnitCode: 'ml', stamp: '', sameName: [] })),
  recipes: vi.fn(() => Promise.resolve({ items: [], total: 0, counts: { pending: 0, review: 0, done: 0 }, totals: {} })),
  products: vi.fn(() => Promise.resolve({ items: [], total: 0, counts: { act: 0, inact: 0 } })),
}));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
vi.mock('../../api/admin', () => ({ adminApi: admin }));
vi.mock('../../api/pos', () => ({
  posApi: { businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' })) },
}));

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><StockPage /></Provider></QueryClientProvider>);
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
