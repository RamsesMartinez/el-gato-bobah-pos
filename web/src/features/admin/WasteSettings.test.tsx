import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { WasteSettings } from './WasteSettings';

const pos = vi.hoisted(() => ({ businessSettings: vi.fn() }));
const waste = vi.hoisted(() => ({ allReasons: vi.fn(), updateReason: vi.fn(), mergeReason: vi.fn(), setThreshold: vi.fn() }));
vi.mock('../../api/pos', () => ({ posApi: pos }));
vi.mock('../../api/waste', async (orig) => ({ ...(await orig<object>()), wasteApi: waste }));

const reason = (id: number, name: string, over: object = {}) => ({
  id, name, description: `Descripción de ${name}`, isStaffMeal: false, archived: false, mergedIntoId: null, uses: 3, ...over,
});

beforeEach(() => {
  pos.businessSettings.mockResolvedValue({ wasteApprovalThreshold: null });
  waste.allReasons.mockResolvedValue([reason(1, 'Caducó'), reason(2, 'Se cayó'), reason(3, 'Se derritió'), reason(4, 'Consumo del personal', { isStaffMeal: true })]);
  waste.updateReason.mockResolvedValue(undefined);
  waste.mergeReason.mockResolvedValue(undefined);
  waste.setThreshold.mockResolvedValue(undefined);
});
afterEach(() => { cleanup(); document.body.innerHTML = ''; vi.clearAllMocks(); });

function montar() {
  render(<QueryClientProvider client={new QueryClient()}><Provider><WasteSettings /></Provider></QueryClientProvider>);
}

describe('motivos y tope de lo que se echó a perder (spec 039, maqueta A)', () => {
  test('el tope se guarda como monto y vacío quiere decir que ninguna espera', async () => {
    montar();
    const input = await screen.findByLabelText('Tope de aprobación');
    fireEvent.change(input, { target: { value: '100' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar tope' }));
    await waitFor(() => expect(waste.setThreshold).toHaveBeenCalledWith('100'));
  });

  test('quitar el tope que ya hay manda vacío: ninguna espera', async () => {
    pos.businessSettings.mockResolvedValue({ wasteApprovalThreshold: '100.00' });
    montar();
    const input = await screen.findByDisplayValue('100.00');
    fireEvent.change(input, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar tope' }));
    await waitFor(() => expect(waste.setThreshold).toHaveBeenCalledWith(null));
  });

  test('editar cambia nombre y descripción, y la marca dice «No cuenta como pérdida del negocio»', async () => {
    montar();
    const row = (await screen.findByText('Se cayó')).closest('[data-testid="motivo"]') as HTMLElement;
    fireEvent.click(within(row).getByRole('button', { name: 'Editar' }));
    fireEvent.change(await screen.findByLabelText('Nombre del motivo'), { target: { value: 'Se tiró' } });
    fireEvent.click(screen.getByRole('button', { name: 'No cuenta como pérdida del negocio' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar motivo' }));
    await waitFor(() => expect(waste.updateReason).toHaveBeenCalledWith(2, expect.objectContaining({ name: 'Se tiró', isStaffMeal: true, archived: false })));
  });

  test('juntar pide a cuál y confirma, y va apartado de «Editar»', async () => {
    montar();
    const row = (await screen.findByText('Se derritió')).closest('[data-testid="motivo"]') as HTMLElement;
    const juntar = within(row).getByRole('button', { name: 'Juntar' });
    expect(juntar.parentElement).toBe(within(row).getByRole('button', { name: 'Editar' }).parentElement);
    fireEvent.click(juntar);
    fireEvent.click(await screen.findByRole('button', { name: /^Caducó/ }));
    expect(waste.mergeReason).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Sí, juntar' }));
    await waitFor(() => expect(waste.mergeReason).toHaveBeenCalledWith(3, 1));
  });
});
