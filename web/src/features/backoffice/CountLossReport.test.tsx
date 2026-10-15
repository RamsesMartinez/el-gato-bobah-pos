import { afterEach, describe, expect, test, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { CountLossReport } from './CountLossReport';

const waste = vi.hoisted(() => ({ countLoss: vi.fn() }));
vi.mock('../../api/waste', async (orig) => ({ ...(await orig<object>()), wasteApi: waste }));

afterEach(() => { cleanup(); vi.clearAllMocks(); });

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><CountLossReport countId={8} /></Provider></QueryClientProvider>);
}

const item = (over: object) => ({
  ingredientId: 1, name: 'Perlas de tapioca', unitCode: 'kg', hasPrevious: true, registered: '3', unexplained: '2',
  surplus: '0', lost: '5', unitCost: '10', lostCost: '50.00', unexplainedCost: '20.00', ...over,
});

describe('faltante sin explicar (spec 039, maqueta B)', () => {
  test('tres cifras arriba y una barra por artículo con lo registrado y lo sin explicar', async () => {
    waste.countLoss.mockResolvedValue({
      firstCount: false,
      items: [item({}), item({ ingredientId: 2, name: 'Azúcar', unitCode: 'g', registered: '0', unexplained: '0', surplus: '600', lost: '0', lostCost: '0.00', unexplainedCost: '0.00' })],
      totals: { lost: '50.00', registered: '30.00', unexplained: '20.00', withoutCost: 0 },
    });
    montar();
    expect(await screen.findByText('Se perdió')).toBeInTheDocument();
    expect(screen.getByText('$50.00')).toBeInTheDocument();
    expect(screen.getByText('$30.00')).toBeInTheDocument();
    expect(screen.getAllByText('$20.00').length).toBeGreaterThan(0);
    const perlas = screen.getByTestId('faltante-1');
    expect(perlas).toHaveTextContent('3 kg registrado');
    expect(perlas).toHaveTextContent('2 kg sin explicar');
    expect(perlas.querySelector('[data-part="registrado"]')).not.toBeNull();
    expect(perlas.querySelector('[data-part="sin-explicar"]')).not.toBeNull();
    // Lo que sobraba se dice así y sin barra: no es un faltante negativo.
    const azucar = screen.getByTestId('faltante-2');
    expect(azucar).toHaveTextContent('Sobraba 600 g');
    expect(azucar.querySelector('[data-part="sin-explicar"]')).toBeNull();
  });

  test('el primer conteo no inventa cifras', async () => {
    waste.countLoss.mockResolvedValue({ firstCount: true, items: [item({ hasPrevious: false })], totals: { lost: '0', registered: '0', unexplained: '0', withoutCost: 0 } });
    montar();
    expect(await screen.findByText(/primer conteo/i)).toBeInTheDocument();
    expect(screen.queryByText('Se perdió')).toBeNull();
  });

  test('sin costo guardado lo dice, no lo pone en $0', async () => {
    waste.countLoss.mockResolvedValue({
      firstCount: false, items: [item({ unitCost: null, lostCost: null, unexplainedCost: null })],
      totals: { lost: '0', registered: '0', unexplained: '0', withoutCost: 1 },
    });
    montar();
    expect(await screen.findByTestId('faltante-1')).toHaveTextContent('sin costo');
    expect(screen.getByText(/1 artículo sin costo/)).toBeInTheDocument();
  });
});
