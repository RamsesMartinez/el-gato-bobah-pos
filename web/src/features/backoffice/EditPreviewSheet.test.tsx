import { describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import type { EditPreview, EditResult } from '../../api/backoffice';
import { ApiError } from '../../api/client';
import { EditPreviewSheet } from './EditPreviewSheet';

// Vista previa de la spec 053 (maqueta «Vista previa», opción B): la tabla antes/después, las
// decisiones dentro de la fila afectada y «Confirmar» bloqueado hasta decidir.

const base = (over: Partial<EditPreview> = {}): EditPreview => ({
  header: [], lines: [], inventory: [], costs: [], decisions: [],
  money: { amountBefore: '67.50', amountAfter: '67.50', paid: '67.50', statusBefore: 'pagada', statusAfter: 'pagada',
    due: '0', overpaid: '0', decisionKey: null },
  fingerprint: 'f1', ...over,
});

const withTwoDecisions = (chosen: Record<string, string>): EditPreview => base({
  inventory: [
    { ingredientId: 7, productId: null, name: 'Vasos 16 oz', baseUnit: 'pieza', before: '40', after: '38', negativeAfter: false, decisionKey: 'count:7' },
    { ingredientId: 8, productId: null, name: 'Popotes', baseUnit: 'pieza', before: '0', after: '1', negativeAfter: false, decisionKey: null },
  ],
  money: { amountBefore: '430', amountAfter: '490', paid: '430', statusBefore: 'pagada', statusAfter: 'pendiente',
    due: '60', overpaid: '0', decisionKey: 'money' },
  decisions: [
    { key: 'count:7', kind: 'count', itemId: null, countId: 3, countedOn: '2026-10-20', options: ['adjust', 'keep'], chosen: chosen['count:7'] ?? null },
    { key: 'money', kind: 'money', itemId: null, countId: null, countedOn: null, options: ['pay', 'leave'], chosen: chosen.money ?? null },
  ],
  fingerprint: JSON.stringify(chosen),
});

function setup(request: (preview: boolean, d: Record<string, string>, f?: string) => Promise<EditResult>) {
  const onDone = vi.fn();
  render(<QueryClientProvider client={new QueryClient()}><Provider><EditPreviewSheet request={request} onClose={() => {}} onDone={onDone} /></Provider></QueryClientProvider>);
  return { onDone };
}

describe('EditPreviewSheet', () => {
  test('con dos decisiones, «Confirmar» espera las dos y el pie dice cuántas faltan', async () => {
    const request = vi.fn(async (preview: boolean, d: Record<string, string>, _f?: string) =>
      ({ preview: withTwoDecisions(d), expense: preview ? null : ({} as never) }) as EditResult);
    const { onDone } = setup(request);
    expect(await screen.findByText('Vasos 16 oz')).toBeTruthy();
    const confirm = screen.getByRole('button', { name: 'Confirmar' }) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
    expect(screen.getByRole('button', { name: /Falta decidir 2/ })).toBeTruthy();
    // La decisión de conteo está en la fila del artículo, con su conteo.
    expect(screen.getByText(/El conteo del 20 oct ya contó/)).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'No mover' }));
    await waitFor(() => expect(screen.getByRole('button', { name: /Falta decidir 1/ })).toBeTruthy());
    fireEvent.click(screen.getByRole('button', { name: 'Dejar pendiente' }));
    await waitFor(() => expect((screen.getByRole('button', { name: 'Confirmar' }) as HTMLButtonElement).disabled).toBe(false));

    fireEvent.click(screen.getByRole('button', { name: 'Confirmar' }));
    await waitFor(() => expect(onDone).toHaveBeenCalled());
    // Confirma con las decisiones y la huella de la ÚLTIMA vista previa.
    const last = request.mock.calls.at(-1)!;
    expect(last[0]).toBe(false);
    expect(last[1]).toEqual({ 'count:7': 'keep', money: 'leave' });
    expect(last[2]).toBe(JSON.stringify({ 'count:7': 'keep', money: 'leave' }));
  });

  test('cada cambio lleva su signo además del color', async () => {
    setup(async () => ({ preview: withTwoDecisions({}), expense: null }));
    expect(await screen.findByText('−2 piezas')).toBeTruthy();
    expect(screen.getByText('+1 pieza')).toBeTruthy();
    expect(screen.getByText(/Falta pagar \$60/)).toBeTruthy();
  });

  test('sin avisos ni decisiones es compacta y se confirma de inmediato', async () => {
    setup(async () => ({ preview: base({ header: [{ field: 'supplier', before: 'Desechables del Centro', after: 'Plásticos del Norte' }] }), expense: null }));
    expect(await screen.findByText('Plásticos del Norte')).toBeTruthy();
    expect(screen.queryByText('Importe')).toBeNull(); // el dinero no cambió: no ocupa filas
    expect((screen.getByRole('button', { name: 'Confirmar' }) as HTMLButtonElement).disabled).toBe(false);
    expect(screen.getByText('Listo para confirmar')).toBeTruthy();
  });

  test('si algo cambió desde la vista previa, la vuelve a pedir y lo dice', async () => {
    let calls = 0;
    const request = vi.fn(async (preview: boolean, _d?: Record<string, string>, _f?: string) => {
      calls++;
      if (!preview && calls === 2) throw new ApiError(409, 'EXPENSE_EDIT_STALE', 'cambió', 'r1');
      return { preview: base({ fingerprint: `f${calls}` }), expense: null } as EditResult;
    });
    const { onDone } = setup(request);
    await screen.findByText('Listo para confirmar');
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar' }));
    expect(await screen.findByText(/Algo cambió mientras revisabas/)).toBeTruthy();
    expect(onDone).not.toHaveBeenCalled();
    // El aviso sale en el mismo render que pide la vista previa nueva; la petición sale después, en
    // el efecto de la consulta. Revisarla sin esperar fallaba 1 de cada 5 corridas.
    await waitFor(() => expect(request).toHaveBeenCalledTimes(3));
    expect(request.mock.calls.at(-1)![0]).toBe(true);
  });
});
