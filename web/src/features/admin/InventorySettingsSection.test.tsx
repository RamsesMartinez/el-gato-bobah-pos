import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { Provider } from '../../components/ui/provider';
import { InventorySettingsSection } from './InventorySettingsSection';

const pos = vi.hoisted(() => ({ businessSettings: vi.fn() }));
const back = vi.hoisted(() => ({ updateInventorySettings: vi.fn(), setExpectedWasteDeducts: vi.fn() }));
vi.mock('../../api/pos', () => ({ posApi: pos }));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

beforeEach(() => {
  pos.businessSettings.mockResolvedValue({ costingMethod: 'last', stockItemLabel: 'artículo' });
  back.updateInventorySettings.mockResolvedValue({});
});
afterEach(() => vi.clearAllMocks());

function montar() {
  render(<QueryClientProvider client={new QueryClient()}><Provider><MemoryRouter><InventorySettingsSection /></MemoryRouter></Provider></QueryClientProvider>);
}

describe('Almacén en Configuración del negocio (spec 038)', () => {
  test('el método de costeo se elige con un toque y por omisión es la última compra', async () => {
    montar();
    const last = await screen.findByRole('button', { name: 'Lo que costó la última compra' });
    expect(last).toHaveAttribute('aria-pressed', 'true');
    fireEvent.click(screen.getByRole('button', { name: 'Promedio de lo que tienes' }));
    await waitFor(() => expect(back.updateInventorySettings).toHaveBeenCalledWith({ costingMethod: 'weighted', stockItemLabel: 'artículo' }));
  });

  test('la palabra se guarda recortada y vacía no se puede guardar', async () => {
    montar();
    const input = await screen.findByLabelText('Palabra del almacén');
    fireEvent.change(input, { target: { value: '   ' } });
    expect(screen.getByRole('button', { name: 'Guardar palabra' })).toBeDisabled();
    fireEvent.change(input, { target: { value: ' insumo ' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar palabra' }));
    await waitFor(() => expect(back.updateInventorySettings).toHaveBeenCalledWith({ costingMethod: 'last', stockItemLabel: 'insumo' }));
  });

  test('sin controles nativos', async () => {
    montar();
    await screen.findByLabelText('Palabra del almacén');
    expect(document.querySelector('select')).toBeNull();
  });

  // Spec 096, subida 2: la opción nace apagada y encenderla manda true; explica qué cambia.
  test('«La merma esperada también descuenta del almacén» se enciende con un toque', async () => {
    back.setExpectedWasteDeducts.mockResolvedValue({ deducts: true });
    montar();
    const sw = await screen.findByRole('checkbox', { name: 'La merma esperada también descuenta del almacén' });
    expect(sw).not.toBeChecked();
    fireEvent.click(sw);
    await waitFor(() => expect(back.setExpectedWasteDeducts).toHaveBeenCalledWith(true));
    expect(screen.getByText(/Las ventas de antes no cambian/)).toBeInTheDocument();
  });
});
