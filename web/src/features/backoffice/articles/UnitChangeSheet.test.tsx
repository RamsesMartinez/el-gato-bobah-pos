import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../../components/ui/provider';
import { ApiError } from '../../../api/client';
import { UnitChangeSheet } from './UnitChangeSheet';

const back = vi.hoisted(() => ({
  units: vi.fn(() => Promise.resolve({ items: [
    { id: 1, code: 'g', name: 'Gramo', kind: 'masa', toBase: '1' }, { id: 2, code: 'kg', name: 'Kilo', kind: 'masa', toBase: '1000' },
    { id: 8, code: 'pieza', name: 'Pieza', kind: 'pieza', toBase: '1' },
  ] })),
  previewUnitChange: vi.fn(),
  changeUnit: vi.fn(() => Promise.resolve()),
}));
vi.mock('../../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

const harina = {
  id: 7, name: 'Harina', isActive: true, baseUnitId: 1, baseUnitCode: 'g', baseUnitKind: 'masa', currentCost: '0.02', minStock: null,
  isPrep: false, categoryId: null, category: null, countPlaceId: null, countPlace: null, onHand: '25000', presentations: [], costSource: 'compra', updatedAt: '',
};

function montar(onDone = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><UnitChangeSheet article={harina} onClose={vi.fn()} onDone={onDone} /></Provider></QueryClientProvider>);
  return onDone;
}

// US5 (FR-009): dos pasos, con el confirmar siempre en el pie y lo que no cuadra primero.
describe('Cambiar en qué se mide', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    back.previewUnitChange.mockResolvedValue({
      path: 'replace', fromUnit: 'g', toUnit: 'pieza', conversions: [
        { key: 'stock:1', group: 'stock', label: 'Existencia · Matriz', from: '25000', to: '5', fits: true },
        { key: 'presentation:3', group: 'presentation', label: 'Empaque: Bulto', from: '25000', to: '5', fits: true },
        { key: 'presentation:4', group: 'presentation', label: 'Empaque: Bolsita', from: '2500', to: '0.5', fits: false },
      ],
    });
  });

  it('«Revisar» se habilita con la equivalencia y manda cuánto de la base nueva vale una de la vieja', async () => {
    montar();
    const revisar = await screen.findByRole('button', { name: 'Revisar cantidades ›' });
    expect(revisar).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'kg' }));
    fireEvent.change(screen.getByLabelText('¿Cuánto es 1 pieza?'), { target: { value: '5' } });
    await waitFor(() => expect(revisar).not.toBeDisabled());
    fireEvent.click(revisar);
    await waitFor(() => expect(back.previewUnitChange).toHaveBeenCalledWith(7, { baseUnitId: 8, factor: '0.0002' }));
  });

  it('en el paso 2 dice que se crea uno nuevo, pone primero lo que no cuadra y aplica las correcciones', async () => {
    const onDone = montar();
    fireEvent.change(await screen.findByLabelText('¿Cuánto es 1 pieza?'), { target: { value: '5000' } });
    fireEvent.click(screen.getByRole('button', { name: 'Revisar cantidades ›' }));
    expect(await screen.findByText(/se crea «Harina» en piezas/)).toBeInTheDocument();
    const rows = screen.getAllByLabelText(/en la unidad nueva/);
    expect(rows[1]).toHaveAccessibleName('Empaque: Bolsita en la unidad nueva');
    expect(screen.getByText('No cuadra: se archiva')).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Existencia · Matriz en la unidad nueva'), { target: { value: '6' } });
    fireEvent.click(screen.getByRole('button', { name: 'Crear el nuevo y archivar este' }));
    await waitFor(() => expect(back.changeUnit).toHaveBeenCalledWith(7, { baseUnitId: 8, factor: '0.0002', overrides: { 'stock:1': '6' } }));
    await waitFor(() => expect(onDone).toHaveBeenCalled());
  });

  it('lo que falta cerrar se dice, y no avanza', async () => {
    back.previewUnitChange.mockRejectedValue(new ApiError(409, 'UNIT_CHANGE_PENDING', 'antes de cambiar en qué se mide, falta recibir 1 compra(s)', 'r'));
    montar();
    fireEvent.change(await screen.findByLabelText('¿Cuánto es 1 pieza?'), { target: { value: '5000' } });
    fireEvent.click(screen.getByRole('button', { name: 'Revisar cantidades ›' }));
    expect(await screen.findByText(/falta recibir 1 compra/)).toBeInTheDocument();
    expect(screen.queryByText(/paso 2 de 2/)).toBeNull();
  });
});
