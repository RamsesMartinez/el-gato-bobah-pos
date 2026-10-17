import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { MemoryRouter, Route, Routes } from 'react-router';

import { Provider } from '../../components/ui/provider';
import { useSessionStore } from '../../stores/session';
import { WasteTab } from './WasteTab';

const waste = vi.hoisted(() => ({ list: vi.fn(), byReason: vi.fn() }));
vi.mock('../../api/waste', async (orig) => ({ ...(await orig<object>()), wasteApi: waste }));
vi.mock('../../api/pos', () => ({ posApi: { businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' })) } }));

const row = (id: number, name: string, reason: string) => ({
  id, itemType: 'ingrediente', itemName: name, quantity: '2', unitCode: 'L', totalCost: '48.00', status: 'applied',
  reason, recordedBy: 'Ana', recordedAt: '2026-10-14T15:00:00Z', canUndo: false,
});

beforeEach(() => {
  waste.byReason.mockResolvedValue([
    { reasonId: 11, name: 'Caducó', isStaffMeal: false, records: 3, totalCost: '184.00', withoutCost: 0 },
    { reasonId: 15, name: 'Consumo del personal', isStaffMeal: true, records: 2, totalCost: '54.00', withoutCost: 0 },
  ]);
  waste.list.mockResolvedValue([row(1, 'Leche entera', 'Caducó'), row(2, 'Frappé de fresa', 'Consumo del personal')]);
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><MemoryRouter initialEntries={['/almacen']}>
    <Routes><Route path="/almacen" element={<WasteTab />} /><Route path="/merma" element={<p>Captura</p>} /></Routes>
  </MemoryRouter></Provider></QueryClientProvider>);
}

describe('pestaña «Se echó a perder» (spec 039, maqueta B)', () => {
  // La prueba con alguien nuevo: buscó dónde registrar en esta pestaña, que se llama igual que la
  // captura, y solo encontró el reporte.
  test('con permiso de registrar, «Registrar» lleva a la captura; sin él, no aparece', async () => {
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'gerente', permissions: ['inventory.write_off'] } as never });
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Registrar' }));
    expect(await screen.findByText('Captura')).toBeInTheDocument();
    cleanup();
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'gerente', permissions: [] } as never });
    montar();
    await screen.findByRole('button', { name: 'Esta semana' });
    expect(screen.queryByRole('button', { name: 'Registrar' })).toBeNull();
  });

  test('fichas por motivo en una fila con la del personal marcada aparte, y debajo los registros', async () => {
    montar();
    const caduco = await screen.findByRole('button', { name: /Caducó.*184\.00/ });
    expect(caduco).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Consumo del personal.*no cuenta como pérdida/i })).toBeInTheDocument();
    expect(await screen.findByText(/Leche entera/)).toBeInTheDocument();
    const fila = caduco.parentElement!;
    expect(getComputedStyle(fila).overflowX).toBe('auto');
  });

  test('tocar una ficha filtra los registros por ese motivo; otra vez, quita el filtro', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Caducó/ }));
    await waitFor(() => expect(waste.list).toHaveBeenLastCalledWith(expect.any(String), expect.any(String), 11));
    fireEvent.click(screen.getByRole('button', { name: /Caducó/ }));
    await waitFor(() => expect(waste.list).toHaveBeenLastCalledWith(expect.any(String), expect.any(String), null));
  });

  test('el rango tiene atajos y no usa el calendario nativo', async () => {
    montar();
    expect(await screen.findByRole('button', { name: 'Esta semana', pressed: true })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Este mes' }));
    await waitFor(() => expect(waste.byReason).toHaveBeenCalledTimes(2));
    expect(document.querySelector('input[type="date"]')).toBeNull();
  });
});
