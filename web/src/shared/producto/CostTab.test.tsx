import { afterEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useState } from 'react';
import { Provider } from '../../components/ui/provider';
import { CostTab } from './CostTab';
import type { ProductCosting } from '../../api/admin';

const admin = vi.hoisted(() => ({ productCosting: vi.fn() }));
vi.mock('../../api/admin', async (orig) => ({ ...(await orig<object>()), adminApi: admin }));
afterEach(() => vi.clearAllMocks());

// El frappé: leche 200 ml $5.00 sin % propio, fresa 100 g $6.00 con 15 % propio. General 5 %.
const frappe: ProductCosting = {
  productId: 1, cost: '11.1500', source: 'receta', base: '11.0000', waste: '1.1500', generalPct: 5, productPct: null,
  lines: [
    { ingredientId: 1, name: 'Leche', baseUnit: 'ml', isPrep: false, qtyBase: '200', unitCost: '0.025', base: '5.0000', pct: 5, pctSource: 'general', waste: '0.2500',
      realWaste: [{ warehouseId: 1, warehouseName: 'Local', isSales: true, wasted: '1', consumed: '50', ratio: 2, state: 'ratio' }] },
    { ingredientId: 2, name: 'Fresa', baseUnit: 'g', isPrep: false, qtyBase: '100', unitCost: '0.06', base: '6.0000', pct: 15, pctSource: 'article', waste: '0.9000', realWaste: [] },
    { ingredientId: 3, name: 'Vaso', baseUnit: 'pieza', isPrep: false, isPackaging: true, qtyBase: '1', unitCost: '2', base: '0.0000', pct: 0, pctSource: 'none', waste: '0.0000', realWaste: [] },
  ],
  components: [],
};

function Harness({ costing, initial = '' }: { costing: ProductCosting; initial?: string }) {
  const [text, setText] = useState(initial);
  admin.productCosting.mockResolvedValue(costing);
  return <CostTab productId={1} text={text} onText={setText} />;
}
function montar(costing: ProductCosting, initial?: string) {
  render(<QueryClientProvider client={new QueryClient()}><Provider><Harness costing={costing} initial={initial} /></Provider></QueryClientProvider>);
}

describe('Ficha del producto › Costo (spec 096, maqueta B)', () => {
  test('cada renglón dice su % y de dónde vino, y la suma es el total', async () => {
    montar(frappe);
    const leche = await screen.findByRole('row', { name: /Leche/ });
    expect(within(leche).getByText('5 %')).toBeInTheDocument();
    expect(within(leche).getByText(/general del negocio/)).toBeInTheDocument();
    expect(within(leche).getByText(/Local 2 %/)).toBeInTheDocument();
    const fresa = screen.getByRole('row', { name: /Fresa/ });
    expect(within(fresa).getByText(/de este artículo/)).toBeInTheDocument();
    expect(screen.getByLabelText('La merma suma')).toHaveTextContent('$1.15');
    expect(screen.getByLabelText('Costo')).toHaveTextContent('$12.15');
  });

  test('teclear 10 en el producto se ve en el acto, y la fresa conserva su 15', async () => {
    montar(frappe);
    await screen.findByRole('row', { name: /Leche/ });
    fireEvent.change(screen.getByLabelText('Merma del producto, en porcentaje'), { target: { value: '10' } });
    const leche = screen.getByRole('row', { name: /Leche/ });
    expect(within(leche).getByText('10 %')).toBeInTheDocument();
    expect(within(leche).getByText(/de este producto/)).toBeInTheDocument();
    expect(within(screen.getByRole('row', { name: /Fresa/ })).getByText('15 %')).toBeInTheDocument();
    // El empaque no toma el % del producto (decisión del dueño, 2026-11-23).
    expect(within(screen.getByRole('row', { name: /Vaso/ })).getByText('0 %')).toBeInTheDocument();
    expect(screen.getByLabelText('La merma suma')).toHaveTextContent('$1.40');
  });

  test('vacío dice que usa la general', async () => {
    montar(frappe);
    expect(await screen.findByText('Usa la general: 5 %')).toBeInTheDocument();
  });

  test('un % inválido lo dice bajo el campo', async () => {
    montar(frappe);
    await screen.findByRole('row', { name: /Leche/ });
    fireEvent.change(screen.getByLabelText('Merma del producto, en porcentaje'), { target: { value: '150' } });
    expect(screen.getByText('Escribe de 0 a 99.99')).toBeInTheDocument();
  });

  test('con costo a mano no ofrece el campo', async () => {
    montar({ ...frappe, source: 'manual', lines: [], base: '8', waste: '0', cost: '8' });
    expect(await screen.findByText('Este producto tiene su costo a mano: la merma no aplica.')).toBeInTheDocument();
    expect(screen.queryByLabelText('Merma del producto, en porcentaje')).toBeNull();
  });

  test('un combo lista sus productos con lo que suma su merma', async () => {
    montar({ ...frappe, source: 'combo', lines: [], components: [{ productId: 3, name: 'Té', qty: 2, cost: '8.4000', waste: '0.4000' }], cost: '8.4', base: '8', waste: '0.4' });
    expect(await screen.findByText('Té × 2')).toBeInTheDocument();
    expect(screen.queryByLabelText('Merma del producto, en porcentaje')).toBeNull();
  });
});
