import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { AddLineSheet } from './AddLineSheet';

const back = vi.hoisted(() => ({ searchArticles: vi.fn(), presentations: vi.fn(), units: vi.fn() }));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
const pos = vi.hoisted(() => ({ businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' })) }));
vi.mock('../../api/pos', () => ({ posApi: pos }));
vi.mock('../../components/Picker', () => ({
  Picker: ({ options, onChange }: { options: { value: string; label: string }[]; onChange: (v: string) => void }) => (
    <div>{options.map((o) => <button key={o.value} type="button" onClick={() => onChange(o.value)}>{o.label}</button>)}</div>
  ),
}));

const salsa = { itemType: 'ingrediente', id: 12, name: 'Salsa de chocolate', unitCode: 'ml', unitKind: 'volumen' };
beforeEach(() => {
  back.presentations.mockResolvedValue([]);
  back.units.mockResolvedValue({ items: [
    { id: 1, code: 'g', name: 'Gramo', kind: 'masa', toBase: '1' }, { id: 2, code: 'kg', name: 'Kilo', kind: 'masa', toBase: '1000' },
    { id: 3, code: 'ml', name: 'Mililitro', kind: 'volumen', toBase: '1' }, { id: 7, code: 'l', name: 'Litro', kind: 'volumen', toBase: '1000' },
  ] });
});
afterEach(() => vi.clearAllMocks());

function montar() {
  const onAdd = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider>
    <AddLineSheet canReceive={false} stockItemLabel="artículo" onClose={vi.fn()} onAdd={onAdd} />
  </Provider></QueryClientProvider>);
  return onAdd;
}

async function enKg(qty: string[]) {
  fireEvent.click(await screen.findByRole('button', { name: 'Salsa de chocolate' }));
  fireEvent.click(await screen.findByRole('button', { name: 'kg' }));
  fireEvent.click(screen.getByLabelText('Cantidad'));
  for (const k of qty) fireEvent.click(screen.getByRole('button', { name: k }));
  fireEvent.click(screen.getByRole('button', { name: 'Listo' }));
}

// US2 escenario 7: agregar un renglón en kg de algo en ml a un gasto guardado se ve igual que en la captura.
describe('Agregar renglón entre peso y líquido (spec 058)', () => {
  test('con densidad dice la cuenta en la misma fila y deja agregar', async () => {
    back.searchArticles.mockResolvedValue({ items: [{ ...salsa, densityGPerMl: '1.1' }] });
    const onAdd = montar();
    await enKg(['4', 'punto', '3']);
    expect(screen.getByText('4,300 g ÷ 1.10 = 3,909 ml')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Fue regalo' }));
    fireEvent.click(screen.getByRole('button', { name: 'Agregar' }));
    expect(onAdd).toHaveBeenCalledWith(expect.objectContaining({ ingredientId: 12, unitId: 2, quantity: '4.3' }));
  });

  // Tres capas (gasto, edición, densidad) no caben: aquí solo se avisa.
  test('sin densidad solo avisa, sin atajo, y no deja agregar', async () => {
    back.searchArticles.mockResolvedValue({ items: [salsa] });
    montar();
    await enKg(['4']);
    const aviso = screen.getByText('Sin densidad para pasar kg a ml');
    // Sin atajo, el aviso dice qué hacer: no es un callejón sin salida.
    expect(screen.getByText(/Pídele a quien configura el almacén que capture la densidad, o elige otra unidad\./)).toBeInTheDocument();
    expect(getComputedStyle(aviso).color).toContain('orange-fg');
    expect(screen.queryByRole('button', { name: 'Capturar la densidad' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Fue regalo' }));
    expect(screen.getByRole('button', { name: 'Agregar' })).toBeDisabled();
  });
});

