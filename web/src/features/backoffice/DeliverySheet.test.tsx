import { beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import type { ExpenseDetail, ExpenseItem } from '../../api/backoffice';
import { DeliverySheet } from './DeliverySheet';

// Hoja de entrega (spec 053, maqueta «Hoja de entrega», opción B + «No llegó»): cada renglón trae lo
// que falta ya puesto, «No llegó» lo deja en 0, y la cantidad se escribe con el teclado en pantalla.

const back = vi.hoisted(() => ({ deliverExpense: vi.fn() }));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
vi.mock('../../components/DatePickerButton', () => ({
  DatePickerButton: ({ value, onChange, label }: { value: string; onChange: (v: string) => void; label?: string }) => (
    <button type="button" aria-label={label ?? 'fecha'} onClick={() => onChange('2026-10-30')}>{value}</button>
  ),
}));

const item = (over: Partial<ExpenseItem>): ExpenseItem => ({
  id: 1, itemType: 'ingrediente', ingredientId: 7, productId: null, itemName: 'Vasos 16 oz', description: 'Vasos',
  quantity: '5', unitCode: 'pieza', qtyReceived: null, unitCost: '10', amount: '50', packQtyInBase: null, applied: '0',
  baseUnitCode: 'pieza', notComing: '0', isGift: false, presentationId: null, deliveries: [], ...over,
});

const expense = (items: ExpenseItem[]): ExpenseDetail => ({
  id: 214, expenseDate: '2026-10-20', receivedAt: null, status: 'pagada', category: 'Desechables', financialGroup: 'operacional',
  supplier: 'Desechables del Centro', amount: '62', currency: 'MXN', description: null, docKind: null, docFolio: null,
  paymentMethod: null, paidAt: null, createdBy: null, itemCount: items.length, items, payments: [], paid: '62',
} as ExpenseDetail);

const preview = { header: [], lines: [], inventory: [], costs: [], decisions: [],
  money: { amountBefore: '62', amountAfter: '62', paid: '62', statusBefore: 'pagada', statusAfter: 'pagada', due: '0', overpaid: '0', decisionKey: null },
  fingerprint: 'f' };

function setup(items: ExpenseItem[]) {
  const onDone = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <Provider><DeliverySheet expense={expense(items)} onClose={() => {}} onDone={onDone} /></Provider>
    </QueryClientProvider>,
  );
  return { onDone };
}

beforeEach(() => {
  back.deliverExpense.mockReset();
  back.deliverExpense.mockResolvedValue({ preview, expense: null });
});

describe('DeliverySheet', () => {
  test('lo que falta llega precargado y se manda tal cual', async () => {
    setup([item({}), item({ id: 2, itemName: 'Popotes', quantity: '1' })]);
    expect((screen.getByLabelText('Cuántos llegaron de Vasos 16 oz') as HTMLInputElement).value).toBe('5');
    expect(screen.getByText('2 de 2 renglones')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Ver efectos' }));
    await waitFor(() => expect(back.deliverExpense).toHaveBeenCalled());
    const [id, body, isPreview] = back.deliverExpense.mock.calls[0];
    expect(id).toBe(214);
    expect(isPreview).toBe(true);
    expect(body.lines).toEqual([{ itemId: 1, qty: '5' }, { itemId: 2, qty: '1' }]);
  });

  test('«No llegó» deja el renglón en 0 y no lo manda; con todo en 0 no hay qué ver', async () => {
    setup([item({})]);
    fireEvent.click(screen.getByRole('button', { name: 'No llegó' }));
    expect((screen.getByLabelText('Cuántos llegaron de Vasos 16 oz') as HTMLInputElement).value).toBe('0');
    expect(screen.getByRole('button', { name: 'Llegó todo' })).toBeTruthy();
    expect(screen.getByText('Nada llegó todavía')).toBeTruthy();
    expect((screen.getByRole('button', { name: 'Ver efectos' }) as HTMLButtonElement).disabled).toBe(true);
  });

  test('tocar la cantidad abre el teclado en pantalla en lugar del pie, y «Listo» lo regresa', () => {
    setup([item({})]);
    const input = screen.getByLabelText('Cuántos llegaron de Vasos 16 oz') as HTMLInputElement;
    expect(input.getAttribute('inputmode')).toBe('none'); // el del sistema no se abre encima
    fireEvent.click(input);
    const pad = screen.getByTestId('teclado');
    expect(screen.queryByRole('button', { name: 'Ver efectos' })).toBeNull();
    fireEvent.click(within(pad).getByRole('button', { name: 'Borrar' }));
    fireEvent.click(within(pad).getByRole('button', { name: '3' }));
    expect(input.value).toBe('3');
    expect(screen.getByText(/Faltarán 2/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Listo' }));
    expect(screen.getByRole('button', { name: 'Ver efectos' })).toBeTruthy();
  });

  test('«Fechas por renglón» manda la fecha de cada renglón', async () => {
    setup([item({})]);
    fireEvent.click(screen.getByRole('button', { name: 'Fechas por renglón' }));
    fireEvent.click(screen.getByRole('button', { name: 'Llegó el día de Vasos 16 oz' }));
    fireEvent.click(screen.getByRole('button', { name: 'Ver efectos' }));
    await waitFor(() => expect(back.deliverExpense).toHaveBeenCalled());
    expect(back.deliverExpense.mock.calls[0][1].lines).toEqual([{ itemId: 1, qty: '5', on: '2026-10-30' }]);
  });

  test('lo que ya llegó completo no se ofrece', () => {
    setup([item({}), item({ id: 3, itemName: 'Tapas', qtyReceived: '5' })]);
    expect(screen.queryByText('Tapas')).toBeNull();
  });
});

// Revisión de tableta y prueba con usuario nuevo: teclear sobre lo precargado lo reemplaza; antes
// «12» + 8 daba «128».
test('la primera tecla reemplaza lo que trae la casilla', () => {
  setup([item({ quantity: '12' })]);
  const input = screen.getByLabelText('Cuántos llegaron de Vasos 16 oz') as HTMLInputElement;
  fireEvent.click(input);
  fireEvent.click(within(screen.getByTestId('teclado')).getByRole('button', { name: '8' }));
  expect(input.value).toBe('8');
  fireEvent.click(within(screen.getByTestId('teclado')).getByRole('button', { name: '0' }));
  expect(input.value).toBe('80');
});
