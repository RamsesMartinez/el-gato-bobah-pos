import { afterEach, describe, expect, test, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ExpensesPage } from './ExpensesPage';

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

afterEach(() => { vi.clearAllMocks(); vi.restoreAllMocks(); });

// CASO 10 DEL LIENZO: el motivo de cancelar un gasto se pedía con el `prompt()` del navegador, que
// en la tableta pinta el sistema con botones fuera de la app. Ahora es una hoja de la app.
describe('cancelar un gasto', () => {
  test('pide el motivo en una hoja de la app, opcional, y no llama a prompt', async () => {
    const prompt = vi.spyOn(window, 'prompt');
    montar();
    await userEvent.click(await screen.findByRole('button', { name: 'Cancelar' }));
    expect(prompt).not.toHaveBeenCalled();
    const campo = await screen.findByLabelText('Motivo');
    await userEvent.type(campo, 'duplicado');
    await userEvent.click(screen.getByRole('button', { name: 'Cancelar gasto' }));
    await waitFor(() => expect(back.cancelExpense).toHaveBeenCalledWith(9, 'duplicado'));
  });

  test('sin motivo también cancela: es opcional', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: 'Cancelar' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Cancelar gasto' }));
    await waitFor(() => expect(back.cancelExpense).toHaveBeenCalledWith(9, ''));
  });

  test('volver no cancela nada', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: 'Cancelar' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Volver' }));
    expect(back.cancelExpense).not.toHaveBeenCalled();
  });
});

// La revisión de tableta: «Pagar» y «Cancelar» medían ~24 px, pegados, uno destructivo.
test('«Pagar» y «Cancelar» miden 44 px', async () => {
  montar();
  for (const n of ['Pagar', 'Cancelar']) {
    const b = await screen.findByRole('button', { name: n });
    expect(parseInt(getComputedStyle(b).minHeight || '0', 10)).toBeGreaterThanOrEqual(44);
  }
  expect(screen.getByRole('button', { name: 'Pagar' }).parentElement)
    .not.toBe(screen.getByRole('button', { name: 'Cancelar' }).parentElement);
});
