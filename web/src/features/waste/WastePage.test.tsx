import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';

import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import { WastePage } from './WastePage';

const waste = vi.hoisted(() => ({
  reasons: vi.fn(), items: vi.fn(), createReason: vi.fn(), record: vi.fn(), today: vi.fn(), undo: vi.fn(),
}));
vi.mock('../../api/waste', async (orig) => ({ ...(await orig<object>()), wasteApi: waste }));

// Doble del Picker sin hoja animada (como en las pruebas del conteo y de compras).
vi.mock('../../components/Picker', async () => {
  const { useState } = await import('react');
  type Opt = { value: string; label: string };
  function Picker({ value, options, onChange, placeholder }: { value: string; options: Opt[]; onChange: (v: string) => void; placeholder?: string }) {
    const [open, setOpen] = useState(false);
    const selected = options.find((o) => o.value === value);
    return (
      <div>
        <button type="button" onClick={() => setOpen(true)}>{selected ? selected.label : placeholder}</button>
        {open && options.map((o) => <button key={o.value} type="button" onClick={() => { onChange(o.value); setOpen(false); }}>{o.label}</button>)}
      </div>
    );
  }
  return { Picker };
});

const items = {
  ingredients: [
    { id: 1, name: 'Leche entera', unitCode: 'ml', units: [{ id: 3, code: 'ml' }, { id: 4, code: 'L' }], packs: [{ id: 9, name: 'Caja', qtyInBase: '12000' }] },
    { id: 2, name: 'Perlas de tapioca', unitCode: 'g', units: [{ id: 1, code: 'g' }, { id: 2, code: 'kg' }], packs: [] },
  ],
  products: [{ id: 50, name: 'Frappé de taro' }],
  recent: [{ itemType: 'ingrediente', id: 2 }],
};

beforeEach(() => {
  try { localStorage.clear(); } catch { /* sin almacenamiento */ }
  waste.reasons.mockResolvedValue([
    { id: 11, name: 'Caducó', description: 'Pasó su fecha', isStaffMeal: false, uses: 9 },
    { id: 12, name: 'Se cayó', description: 'Accidente', isStaffMeal: false, uses: 3 },
  ]);
  waste.items.mockResolvedValue(items);
  waste.today.mockResolvedValue([]);
  waste.record.mockResolvedValue({ id: 70, status: 'applied', totalCost: '48.00' });
  waste.undo.mockResolvedValue(undefined);
});
afterEach(() => {
  cleanup();
  document.body.innerHTML = '';
  vi.clearAllMocks();
});

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><MemoryRouter><WastePage /></MemoryRouter></Provider></QueryClientProvider>);
}
const click = (name: string | RegExp) => fireEvent.click(screen.getByRole('button', { name }));
const tecla = (k: string) => fireEvent.click(screen.getByRole('button', { name: k }));

describe('Se echó a perder (spec 039, maqueta D)', () => {
  test('llega «Una pantalla»: motivo, artículo reciente, cantidad y guardar en 3 toques más los dígitos', async () => {
    montar();
    expect(await screen.findByRole('button', { name: 'Una pantalla', pressed: true })).toBeInTheDocument();
    await screen.findByRole('button', { name: 'Caducó' });
    click('Caducó');
    click('Perlas de tapioca');
    tecla('2');
    click('Guardar');
    await waitFor(() => expect(waste.record).toHaveBeenCalledWith(expect.objectContaining({ reasonId: 11, ingredientId: 2, qty: '2' })));
  });

  test('en empaque manda el empaque; en la unidad grande, la unidad', async () => {
    montar();
    await screen.findByRole('button', { name: 'Caducó' });
    click('Caducó');
    click(/Buscar artículo/);
    click('Leche entera');
    click('Caja');
    tecla('1');
    click('Guardar');
    await waitFor(() => expect(waste.record).toHaveBeenLastCalledWith(expect.objectContaining({ ingredientId: 1, presentationId: 9, qty: '1' })));
    click('Caducó');
    click(/Buscar artículo/);
    click('Leche entera');
    click('L');
    tecla('2');
    click('Guardar');
    await waitFor(() => expect(waste.record).toHaveBeenLastCalledWith(expect.objectContaining({ ingredientId: 1, unitId: 4, qty: '2' })));
  });

  test('la cantidad no abre el teclado del sistema y no hay controles nativos', async () => {
    montar();
    const cantidad = await screen.findByLabelText('Cantidad');
    expect(cantidad).toHaveAttribute('inputmode', 'none');
    expect(document.querySelector('select')).toBeNull();
    expect(document.querySelector('input[type="date"], input[type="time"]')).toBeNull();
  });

  test('si queda por aprobar, lo dice y no dice que ya restó', async () => {
    waste.record.mockResolvedValue({ id: 71, status: 'pending', totalCost: '285.00' });
    montar();
    await screen.findByRole('button', { name: 'Caducó' });
    click('Caducó');
    click('Perlas de tapioca');
    tecla('3');
    click('Guardar');
    expect(await screen.findByText(/espera aprobación/i)).toBeInTheDocument();
  });

  test('«Paso a paso» guía motivo, qué y cuánto, y la tableta lo recuerda', async () => {
    montar();
    await screen.findByRole('button', { name: 'Paso a paso' });
    click('Paso a paso');
    click('Se cayó');
    click('Perlas de tapioca');
    tecla('5');
    click('Guardar');
    await waitFor(() => expect(waste.record).toHaveBeenCalledWith(expect.objectContaining({ reasonId: 12, ingredientId: 2, qty: '5' })));
    cleanup();
    montar();
    expect(await screen.findByRole('button', { name: 'Paso a paso', pressed: true })).toBeInTheDocument();
  });

  test('un motivo nuevo pide descripción; si ya existe, se elige el que existe', async () => {
    waste.createReason.mockRejectedValue(new ApiError(409, 'CONFLICT', 'ese motivo ya existe', 'r', { existingId: 11 }));
    montar();
    await screen.findByRole('button', { name: 'Otro motivo…' });
    click('Otro motivo…');
    fireEvent.change(await screen.findByLabelText('Nombre del motivo'), { target: { value: 'caduco' } });
    expect(screen.getByRole('button', { name: 'Agregar motivo' })).toBeDisabled();
    fireEvent.change(screen.getByLabelText('Descripción'), { target: { value: 'Se pasó' } });
    click('Agregar motivo');
    expect(await screen.findByRole('button', { name: 'Caducó', pressed: true })).toBeInTheDocument();
  });

  test('«Hoy» deshace con confirmación, con el botón a la derecha del renglón', async () => {
    waste.today.mockResolvedValue([{ id: 5, itemType: 'ingrediente', itemName: 'Leche entera', quantity: '1', unitCode: 'L', totalCost: '24.00', status: 'applied', reason: 'Caducó', recordedBy: 'Ana', recordedAt: '2026-10-15T15:00:00Z', canUndo: true }]);
    montar();
    await screen.findByRole('button', { name: /^Hoy/ });
    click(/^Hoy/);
    const row = (await screen.findByText(/Leche entera/)).closest('[data-testid="renglon-merma"]') as HTMLElement;
    fireEvent.click(within(row).getByRole('button', { name: 'Deshacer' }));
    expect(waste.undo).not.toHaveBeenCalled();
    const botones = within(row).getAllByRole('button').map((b) => b.textContent);
    expect(botones[botones.length - 1]).toBe('No');
    click('Sí, deshacer');
    await waitFor(() => expect(waste.undo).toHaveBeenCalledWith(5));
  });

  // Si alguien más ya la deshizo, el 409 se quedaba callado y los botones de confirmar seguían ahí:
  // quien opera no sabía si se había deshecho.
  test('si no se puede deshacer, lo dice y quita la confirmación', async () => {
    waste.today.mockResolvedValue([{ id: 6, itemType: 'ingrediente', itemName: 'Tortillas', quantity: '1000', unitCode: 'g', totalCost: '13.00', status: 'applied', reason: 'Se cayó', recordedBy: 'Ana', recordedAt: '2026-10-15T15:00:00Z', canUndo: true }]);
    waste.undo.mockRejectedValue(new ApiError(409, 'CONFLICT', 'ya se deshizo', 'r'));
    montar();
    await screen.findByRole('button', { name: /^Hoy/ });
    click(/^Hoy/);
    const row = (await screen.findByText(/Tortillas/)).closest('[data-testid="renglon-merma"]') as HTMLElement;
    fireEvent.click(within(row).getByRole('button', { name: 'Deshacer' }));
    click('Sí, deshacer');
    expect(await screen.findByText('ya se deshizo')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sí, deshacer' })).not.toBeInTheDocument();
  });
});
