import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { CostSettingsSection } from './CostSettingsSection';
import { ApiError } from '../../api/client';

const pos = vi.hoisted(() => ({ businessSettings: vi.fn() }));
const back = vi.hoisted(() => ({ setGeneralExpectedWaste: vi.fn() }));
vi.mock('../../api/pos', () => ({ posApi: pos }));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

beforeEach(() => {
  pos.businessSettings.mockResolvedValue({ expectedWastePct: null });
  back.setGeneralExpectedWaste.mockResolvedValue({ pct: '5' });
});
afterEach(() => vi.clearAllMocks());

function montar() {
  render(<QueryClientProvider client={new QueryClient()}><Provider><CostSettingsSection /></Provider></QueryClientProvider>);
}

describe('Merma general en Ajustes › Costos (spec 096)', () => {
  test('arranca sin merma y Guardar no hace nada sin cambio', async () => {
    montar();
    const input = await screen.findByLabelText('Merma al preparar, en porcentaje');
    expect(input).toHaveValue('');
    expect(input).toHaveAttribute('placeholder', 'Sin merma');
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
  });

  test('teclear 5 y Guardar manda 5', async () => {
    montar();
    fireEvent.change(await screen.findByLabelText('Merma al preparar, en porcentaje'), { target: { value: '5' } });
    // El refetch tarda: mientras, la caché ya trae lo guardado.
    pos.businessSettings.mockReturnValue(new Promise(() => {}));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(back.setGeneralExpectedWaste).toHaveBeenCalledWith(5));
    // Guardado, el campo se queda en 5 aunque el refetch todavía no llegue.
    await waitFor(() => expect(screen.getByLabelText('Merma al preparar, en porcentaje')).toHaveValue('5'));
  });

  test('vaciar una merma guardada manda null, nunca 0', async () => {
    pos.businessSettings.mockResolvedValue({ expectedWastePct: '5.00' });
    montar();
    const input = await screen.findByLabelText('Merma al preparar, en porcentaje');
    await waitFor(() => expect(input).toHaveValue('5'));
    fireEvent.change(input, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(back.setGeneralExpectedWaste).toHaveBeenCalledWith(null));
  });

  test('un valor fuera de rango no se manda y dice por qué', async () => {
    montar();
    fireEvent.change(await screen.findByLabelText('Merma al preparar, en porcentaje'), { target: { value: '120' } });
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
    expect(screen.getByText('Escribe de 0 a 99.99')).toBeInTheDocument();
  });

  test('un rechazo del servidor se dice con su mensaje, no con uno inventado', async () => {
    back.setGeneralExpectedWaste.mockRejectedValue(new ApiError(409, 'conflict', 'Captura primero los datos del negocio', 'r'));
    montar();
    fireEvent.change(await screen.findByLabelText('Merma al preparar, en porcentaje'), { target: { value: '5' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    expect(await screen.findByText('Captura primero los datos del negocio')).toBeInTheDocument();
  });
});
