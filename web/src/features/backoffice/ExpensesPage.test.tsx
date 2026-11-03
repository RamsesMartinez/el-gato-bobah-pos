import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ExpensesPage } from './ExpensesPage';
import { useSessionStore } from '../../stores/session';

const back = vi.hoisted(() => ({
  expenses: vi.fn(() => Promise.resolve({
    items: [{
      id: 9, expenseDate: '2026-10-08', receivedAt: '2026-10-08', status: 'pendiente', category: 'Insumos',
      financialGroup: 'operacional', supplier: 'Costco', amount: '450.00', currency: 'MXN', description: null,
      docKind: null, docFolio: null, paymentMethod: null, paidAt: null, createdBy: 'Ana', itemCount: 0,
    }],
    total: 1,
  })),
  cancelExpense: vi.fn(() => Promise.resolve()),
  cashRegisters: vi.fn(() => Promise.resolve({ items: [] })),
}));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
vi.mock('../../api/pos', () => ({
  posApi: {
    paymentMethods: vi.fn(() => Promise.resolve({ items: [] })),
    businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' })),
  },
}));

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><ExpensesPage /></Provider></QueryClientProvider>);
}

beforeEach(() => useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'gerente', permissions: ['expenses.cancel'] } as never }));
afterEach(() => { vi.clearAllMocks(); vi.restoreAllMocks(); });

// CASO 10 DEL LIENZO: el motivo de cancelar un gasto se pedía con el `prompt()` del navegador, que
// en la tableta pinta el sistema con botones fuera de la app. Ahora es una hoja de la app.
describe('cancelar un gasto', () => {
  test('pide el motivo en una hoja de la app, opcional, y no llama a prompt', async () => {
    const prompt = vi.spyOn(window, 'prompt');
    montar();
    await userEvent.click(await screen.findByRole('button', { name: 'Anular' }));
    expect(prompt).not.toHaveBeenCalled();
    const campo = await screen.findByLabelText('Motivo');
    await userEvent.type(campo, 'duplicado');
    await userEvent.click(screen.getByRole('button', { name: 'Sí, anular' }));
    await waitFor(() => expect(back.cancelExpense).toHaveBeenCalledWith(9, 'duplicado'));
  });

  // #216: el motivo es obligatorio; el gasto sale de los reportes y la bitácora es lo único que dirá por qué.
  test('sin motivo no cancela', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: 'Anular' }));
    expect(await screen.findByRole('button', { name: 'Sí, anular' })).toBeDisabled();
    expect(back.cancelExpense).not.toHaveBeenCalled();
  });

  test('sin el permiso no se ofrece anular', async () => {
    useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'gerente', permissions: [] } as never });
    montar();
    expect(await screen.findByRole('button', { name: 'Pagar' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Anular' })).toBeNull();
  });

  test('volver no cancela nada', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: 'Anular' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Volver' }));
    expect(back.cancelExpense).not.toHaveBeenCalled();
  });
});

// La revisión de tableta: «Pagar» y «Cancelar» medían ~24 px, pegados, uno destructivo.
test('«Pagar» y «Cancelar» miden 44 px', async () => {
  montar();
  for (const n of ['Pagar', 'Anular']) {
    const b = await screen.findByRole('button', { name: n });
    expect(parseInt(getComputedStyle(b).minHeight || '0', 10)).toBeGreaterThanOrEqual(44);
  }
  expect(screen.getByRole('button', { name: 'Pagar' }).parentElement)
    .not.toBe(screen.getByRole('button', { name: 'Anular' }).parentElement);
});

// Revisión de tableta del 038: los filtros de estado eran `size="sm"` (~32 px), y el nuevo de
// compras por llegar se agregó igual.
test('los filtros de la lista miden 44 px', async () => {
  montar();
  for (const n of ['Todos', 'Pendientes', 'Con pendientes de llegar']) {
    const b = await screen.findByRole('button', { name: n });
    expect(parseInt(getComputedStyle(b).minHeight || '0', 10)).toBeGreaterThanOrEqual(44);
  }
});

// Prueba con usuario nuevo (#216): el botón dice «Anular» y la lista decía «Cancelada»; una sola palabra.
test('la lista llama «Anuladas» a los gastos anulados', async () => {
  montar();
  expect(await screen.findByRole('button', { name: 'Anuladas' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Canceladas' })).toBeNull();
});
