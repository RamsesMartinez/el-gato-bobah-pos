import { beforeEach, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../../components/ui/provider';
import { useSessionStore } from '../../../stores/session';
import type { ExpiryLot } from '../../../api/backoffice';
import { ExpiriesTab } from './ExpiriesTab';

const back = vi.hoisted(() => ({ expiries: vi.fn(), warehouses: vi.fn() }));
const waste = vi.hoisted(() => ({ reasons: vi.fn(), record: vi.fn() }));
vi.mock('../../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
vi.mock('../../../api/waste', async (orig) => ({ ...(await orig<object>()), wasteApi: waste }));

const lot = (over: Partial<ExpiryLot>): ExpiryLot => ({
  ingredientId: 1, name: 'Queso Oaxaca', unitCode: 'kg', warehouseId: 1, warehouseName: 'Local', expiresOn: '2026-11-28',
  qty: '2', daysLeft: 5, expired: false, dailyUse: '0.3', noRecentUse: false, finishesInDays: null, leftoverAtExpiry: '0.5',
  pendingWaste: '0', ...over,
});

function setup(lots: ExpiryLot[], perms = ['inventory.view', 'inventory.write_off']) {
  useSessionStore.setState({ user: { id: 1, name: 'Lupe', permissions: perms } } as never);
  back.expiries.mockResolvedValue({ today: '2026-11-23', counts: { expired: 1, within7: 1, within30: 0, within60: 0 }, lots });
  render(<QueryClientProvider client={new QueryClient()}><Provider><ExpiriesTab /></Provider></QueryClientProvider>);
}

beforeEach(() => {
  back.expiries.mockReset();
  back.warehouses.mockResolvedValue({ items: [{ id: 1, name: 'Local', isSales: true }, { id: 2, name: 'Bodega', isSales: false }] });
  waste.reasons.mockResolvedValue([{ id: 7, name: 'Caducó', description: '', isStaffMeal: false, uses: 0 }, { id: 8, name: 'Se cayó', description: '', isStaffMeal: false, uses: 0 }]);
  waste.record.mockReset();
  waste.record.mockResolvedValue({ id: 1, status: 'applied', totalCost: null });
});

test('agrupa por plazo y cada lote dice en una frase qué pasa', async () => {
  setup([
    lot({ name: 'Crema', expiresOn: '2026-11-20', daysLeft: -3, expired: true, leftoverAtExpiry: '1.5' }),
    lot({}),
    lot({ name: 'Leche', daysLeft: 3, leftoverAtExpiry: null, finishesInDays: 2 }),
    lot({ name: 'Mantequilla', daysLeft: 80, noRecentUse: true, dailyUse: null }),
  ]);
  expect(await screen.findByTestId('grupo-Ya caducó')).toBeTruthy();
  expect(screen.getByText('Caducó: mándalo a merma')).toBeTruthy();
  expect(screen.getByText('No se termina antes de caducar: quedan ~0.5 kg')).toBeTruthy();
  expect(screen.getByText('Se termina en ~2 días')).toBeTruthy();
  // Más adelante va cerrado.
  expect(screen.queryByText('Mantequilla')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Ver' }));
  expect(screen.getByText('No se ha vendido en 30 días')).toBeTruthy();
});

test('sin lotes dice cómo registrarlos', async () => {
  setup([]);
  expect(await screen.findByTestId('sin-caducidades')).toBeTruthy();
});

test('lo caducado va a merma en dos toques, del lote y con «Caducó» ya elegido', async () => {
  setup([lot({ expiresOn: '2026-11-20', daysLeft: -3, expired: true, warehouseId: 2, warehouseName: 'Bodega' })]);
  fireEvent.click(await screen.findByRole('button', { name: 'A merma' }));
  const send = await screen.findByRole('button', { name: 'Mandar a merma' });
  await waitFor(() => expect(send).toBeEnabled());
  fireEvent.click(send);
  await waitFor(() => expect(waste.record).toHaveBeenCalled());
  expect(waste.record.mock.calls[0][0]).toEqual({ reasonId: 7, ingredientId: 1, qty: '2', warehouseId: 2, lotExpiresOn: '2026-11-20' });
});

test('un lote vigente pide el motivo antes de mandarlo', async () => {
  setup([lot({})]);
  fireEvent.click(await screen.findByRole('button', { name: 'A merma' }));
  expect(await screen.findByRole('button', { name: 'Mandar a merma' })).toBeDisabled();
});

test('con merma por aprobar el renglón lo dice y no deja mandarla otra vez', async () => {
  setup([lot({ expired: true, daysLeft: -1, pendingWaste: '2' })]);
  const row = (await screen.findAllByTestId('lote'))[0];
  expect(within(row).getByText('Merma pendiente de aprobación')).toBeTruthy();
  expect(within(row).getByRole('button', { name: 'A merma' })).toBeDisabled();
});

test('sin permiso de merma no hay botón', async () => {
  setup([lot({ expired: true, daysLeft: -1 })], ['inventory.view']);
  await screen.findAllByTestId('lote');
  expect(screen.queryByRole('button', { name: 'A merma' })).toBeNull();
});
