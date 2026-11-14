import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import userEvent from '@testing-library/user-event';

import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import type { LowStockConfig } from '../../api/notifications';
import { SeccionAlertas } from './SeccionAlertas';

const api = vi.hoisted(() => ({ get: vi.fn(), preview: vi.fn(), save: vi.fn(), asked: vi.fn(), list: vi.fn(), batch: vi.fn() }));
vi.mock('../../api/notifications', async (orig) => ({ ...(await orig<object>()), lowStockApi: api }));

const cfg = (o: Partial<LowStockConfig> = {}): LowStockConfig => ({
  enabled: false, onMin: true, onOut: true, banner: false, minStock: null, branchMinStock: null, coverageDays: null,
  asked: true, onHand: '1200', unitCode: 'g', branches: 1, updatedAt: 'v1', ...o,
});

function montar(canConfigure = true, onSaved?: (v: string) => void) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider>
    <SeccionAlertas itemType="ingrediente" id={1} name="Queso crema" kind="masa" canConfigure={canConfigure} onSaved={onSaved} />
  </Provider></QueryClientProvider>);
}

beforeEach(() => {
  api.get.mockResolvedValue(cfg());
  api.asked.mockResolvedValue(undefined);
  api.preview.mockResolvedValue({ effect: { opens: ['stock_low'], closes: [] }, onHand: '1200', min: '2000', needsCount: false });
  api.save.mockResolvedValue({ effect: { opens: ['stock_low'], closes: [] }, onHand: '1200', min: '2000', needsCount: false });
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

describe('Alertas en la ficha', () => {
  test('el renglón dice el estado en palabras', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000' }));
    montar();
    expect(await screen.findByText('Avisa al llegar a 2 kg y al acabarse')).toBeTruthy();
  });

  test('una ficha que nunca preguntó pregunta, y «No, por ahora» no vuelve', async () => {
    api.get.mockResolvedValue(cfg({ asked: false }));
    montar();
    expect(await screen.findByText('¿Quieres recibir alertas de stock bajo de Queso crema?')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'No, por ahora' }));
    await waitFor(() => expect(api.asked).toHaveBeenCalledWith(1));
  });

  test('sin poder configurar no pregunta ni ofrece «Configurar»', async () => {
    api.get.mockResolvedValue(cfg({ asked: false }));
    montar(false);
    await screen.findByText(/Alertas:/);
    expect(screen.queryByText(/¿Quieres recibir alertas/)).toBeNull();
    expect(screen.queryByRole('button', { name: 'Configurar ›' })).toBeNull();
  });

  test('guardar muestra primero lo que pasará y manda el mínimo en la unidad base', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    await userEvent.click(screen.getByRole('checkbox', { name: /Enviar alerta de stock bajo/ }));
    const min = screen.getByLabelText(/Mínimo/);
    fireEvent.change(min, { target: { value: '2' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalledWith('ingrediente', 1, expect.objectContaining({ enabled: true, minStock: '2000' })));
    expect(api.save).not.toHaveBeenCalled();
    expect(await screen.findByText(/se enviará un aviso al llegar al mínimo al guardar/)).toBeTruthy();
    fireEvent.click(screen.getAllByRole('button', { name: 'Guardar', hidden: true }).at(-1)!);
    await waitFor(() => expect(api.save).toHaveBeenCalled());
  });

  test('un mínimo vacío viaja como «sin mínimo», nunca como 0', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000' }));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    const min = screen.getByLabelText(/Mínimo/);
    fireEvent.change(min, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalledWith('ingrediente', 1, expect.objectContaining({ minStock: null })));
  });

  // Sin mínimo, «llegue al mínimo» se ve deshabilitada pero seguía viajando encendida.
  test('sin mínimo, «llegue al mínimo» viaja apagada', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000', onMin: true }));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    fireEvent.change(screen.getByLabelText(/Mínimo/), { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalledWith('ingrediente', 1, expect.objectContaining({ minStock: null, onMin: false })));
  });

  // Sin la versión, otra tableta guardaba mínimo 8 y ésta, al cambiar solo la franja, lo regresaba a 5.
  test('la vista previa y el guardado llevan la versión que se vio; si cambió, se vuelve a leer y se dice', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000' }));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalledWith('ingrediente', 1, expect.objectContaining({ expectedUpdatedAt: 'v1' })));
    api.save.mockRejectedValueOnce(new ApiError(409, 'STALE', 'cambió', 'r'));
    const llamadas = api.get.mock.calls.length;
    await screen.findByText(/Se actualizará/);
    fireEvent.click(screen.getAllByRole('button', { name: 'Guardar', hidden: true }).at(-1)!);
    await waitFor(() => expect(api.save).toHaveBeenCalledWith('ingrediente', 1, expect.objectContaining({ expectedUpdatedAt: 'v1' })));
    expect(await screen.findByText(/Alguien más cambió estas alertas/)).toBeTruthy();
    await waitFor(() => expect(api.get.mock.calls.length).toBeGreaterThan(llamadas));
  });

  // La ficha decía «avisa al llegar a 3» y el aviso salía en 10: el mínimo de la sucursal gana.
  test('con mínimo de sucursal, el renglón y la confirmación dicen el mínimo que de verdad se usa', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '3000', branchMinStock: '10000' }));
    api.preview.mockResolvedValue({ effect: { opens: ['stock_low'], closes: [] }, onHand: '1200', min: '10000', needsCount: false });
    montar();
    expect(await screen.findByText(/En esta sucursal avisa al llegar a 10 kg/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Configurar ›' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    expect(await screen.findByText(/avisa al llegar a 10 kg/)).toBeTruthy();
  });

  test('los días de cobertura van de 1 a 90 y se dice antes de la vista previa', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000' }));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    fireEvent.change(screen.getByLabelText('Días de cobertura'), { target: { value: '95' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    expect(await screen.findByText(/de 1 a 90 días/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Días de cobertura'), { target: { value: '0' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    expect(await screen.findByText(/de 1 a 90 días/)).toBeTruthy();
    expect(api.preview).not.toHaveBeenCalled();
  });

  // Guardar las alertas cambia la versión de la ficha: la ficha la necesita para que su autoguardado
  // no choque con un «cambió mientras la veías» falso.
  test('al guardar avisa a la ficha la versión nueva', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000' }));
    api.save.mockResolvedValue({ effect: { opens: [], closes: [] }, onHand: '1200', min: '2000', needsCount: false, updatedAt: 'v2' });
    const onSaved = vi.fn();
    montar(true, onSaved);
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await screen.findByText(/Se actualizará/);
    fireEvent.click(screen.getAllByRole('button', { name: 'Guardar', hidden: true }).at(-1)!);
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith('v2'));
  });
});
