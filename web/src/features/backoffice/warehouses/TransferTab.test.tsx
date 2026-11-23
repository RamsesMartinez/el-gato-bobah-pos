import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../../components/ui/provider';
import { ApiError } from '../../../api/client';
import { TransferTab } from './TransferTab';

const back = vi.hoisted(() => ({ stockLevels: vi.fn(), createTransfer: vi.fn() }));
vi.mock('../../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
const toasts = vi.hoisted(() => ({ create: vi.fn() }));
vi.mock('../../../components/ui/toaster', () => ({ toaster: toasts }));

const WH = [
  { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 2 },
  { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 2 },
];
const lv = (name: string, id: number, onHand: string, unit = 'ml') =>
  ({ item_type: 'ingrediente', item_name: name, ingredient_id: id, product_id: null, on_hand: onHand, unit_code: unit, min_stock: null, is_active: true });

beforeEach(() => {
  try { sessionStorage.clear(); } catch { /* sin almacenamiento */ }
  back.stockLevels.mockImplementation((w?: number) => Promise.resolve({ items: w === 2
    ? [lv('Leche entera', 7, '20000'), lv('Azúcar', 8, '25000', 'g'), lv('Vacío', 9, '0')]
    : [lv('Leche entera', 7, '-2000'), lv('Azúcar', 8, '4500', 'g')] }));
  back.createTransfer.mockResolvedValue({ id: 99 });
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><TransferTab warehouses={WH} /></Provider></QueryClientProvider>);
}
const fila = async (name: string) => (await screen.findByText(name)).closest('tr') as HTMLElement;

describe('traspaso entre almacenes (spec 094, maqueta B)', () => {
  test('por omisión de la bodega al local, con lo que hay en los dos lado a lado', async () => {
    montar();
    expect(within(await screen.findByRole('group', { name: 'De' })).getByRole('button', { name: /Bodega/ })).toBeInTheDocument();
    expect(within(screen.getByRole('group', { name: 'A' })).getByRole('button', { name: /Local/ })).toBeInTheDocument();
    const leche = await fila('Leche entera');
    expect(leche.textContent).toContain('20 L');
    expect(leche.textContent).toContain('-2 L');
    // Lo que el origen no tiene no se ofrece.
    expect(screen.queryByText('Vacío')).toBeNull();
  });

  test('invertir cambia el origen y el destino', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Invertir' }));
    expect(within(await screen.findByRole('group', { name: 'De' })).getByRole('button', { name: /Local/ })).toBeInTheDocument();
    await waitFor(() => expect(back.stockLevels).toHaveBeenCalledWith(1));
  });

  // La cantidad se teclea con el teclado de la pantalla, no el del sistema (revisión de pantallas, R12).
  test('tocar la cantidad abre el teclado de la pantalla', async () => {
    montar();
    const leche = await fila('Leche entera');
    const box = within(leche).getByLabelText('Cuánto de Leche entera');
    expect(box).toHaveAttribute('inputmode', 'none');
    fireEvent.click(box);
    expect(await screen.findByTestId('teclado')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '6' }));
    expect(box).toHaveValue('6');
  });

  test('manda el traspaso en unidad base y limpia al terminar', async () => {
    montar();
    const leche = await fila('Leche entera');
    fireEvent.click(within(leche).getByRole('button', { name: 'Más' }));
    fireEvent.click(within(leche).getByRole('button', { name: 'Más' }));
    fireEvent.click(screen.getByRole('button', { name: 'Traspasar 1 artículo' }));
    await waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(1));
    const body = back.createTransfer.mock.calls[0][0];
    expect(body).toMatchObject({ fromWarehouseId: 2, toWarehouseId: 1,
      lines: [{ itemType: 'ingrediente', ingredientId: 7, qty: '2000' }] });
    expect(body.clientKey).toMatch(/^[0-9a-f-]{36}$/);
    await waitFor(() => expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: expect.stringMatching(/^Listo: .+ de Bodega a Local$/) })));
    expect(within(await fila('Leche entera')).getByLabelText('Cuánto de Leche entera')).toHaveValue('');
  });

  // Un reintento (red caída, o confirmar el negativo) reusa la clave: el servidor no duplica.
  // Prueba con alguien nuevo (cierre del #65): Ajonjolí se veía «105 g» y la cantidad se tecleaba en
  // kg; «2» llevaba 2 kg de algo que solo tenía 105 g.
  test('la cantidad se teclea en la unidad en que se ve la existencia del origen', async () => {
    back.stockLevels.mockImplementation((w?: number) => Promise.resolve({ items: w === 2
      ? [lv('Ajonjolí', 11, '105', 'g'), lv('Azúcar', 8, '25000', 'g')] : [] }));
    montar();
    const ajonjoli = await fila('Ajonjolí');
    expect(ajonjoli.textContent).toContain('105 g');
    expect(within(ajonjoli).getByText('g')).toBeInTheDocument();
    expect(within(await fila('Azúcar')).getByText('kg')).toBeInTheDocument();
    fireEvent.click(within(ajonjoli).getByRole('button', { name: 'Más' }));
    fireEvent.click(within(ajonjoli).getByRole('button', { name: 'Más' }));
    fireEvent.click(screen.getByRole('button', { name: 'Traspasar 1 artículo' }));
    await waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(1));
    expect(back.createTransfer.mock.calls[0][0].lines).toEqual([{ itemType: 'ingrediente', ingredientId: 11, qty: '2' }]);
    await waitFor(() => expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: 'Listo: 2 g de Ajonjolí de Bodega a Local' })));
  });

  test('dejar el origen en negativo pide confirmar y reenvía con la misma clave', async () => {
    back.createTransfer.mockRejectedValueOnce(new ApiError(409, 'NEEDS_CONFIRM', 'x', '',
      { lines: [{ itemType: 'ingrediente', id: 8, available: '25000' }] }));
    montar();
    const azucar = await fila('Azúcar');
    fireEvent.click(within(azucar).getByLabelText('Cuánto de Azúcar'));
    fireEvent.click(await screen.findByRole('button', { name: '3' }));
    fireEvent.click(screen.getByRole('button', { name: '0' }));
    fireEvent.click(screen.getByRole('button', { name: 'Listo' }));
    fireEvent.click(screen.getByRole('button', { name: 'Traspasar 1 artículo' }));
    expect(await screen.findByText(/Azúcar: hay 25 kg/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Traspasar de todos modos' }));
    await waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(2));
    const [first, second] = back.createTransfer.mock.calls.map((c) => c[0]);
    expect(second.confirmNegative).toBe(true);
    expect(second.clientKey).toBe(first.clientKey);
  });

  test('un error de red conserva la clave para el reintento', async () => {
    back.createTransfer.mockRejectedValueOnce(new Error('red'));
    montar();
    const leche = await fila('Leche entera');
    fireEvent.click(within(leche).getByRole('button', { name: 'Más' }));
    fireEvent.click(screen.getByRole('button', { name: 'Traspasar 1 artículo' }));
    expect(await screen.findByText('No se pudo traspasar. Vuelve a intentar.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Traspasar 1 artículo' }));
    await waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(2));
    expect(back.createTransfer.mock.calls[1][0].clientKey).toBe(back.createTransfer.mock.calls[0][0].clientKey);
  });

  // La clave ya se usó para otro traspaso (se mandó, la respuesta se perdió y luego se cambió la
  // cantidad): reintentar con la misma clave fallaría siempre. Se renueva y se avisa que el primero
  // pudo haber entrado.
  test('una clave ya usada se renueva y avisa que el traspaso ya se había hecho', async () => {
    back.createTransfer.mockRejectedValueOnce(new ApiError(409, 'TRANSFER_KEY_REUSED', 'x', ''));
    montar();
    const leche = await fila('Leche entera');
    fireEvent.click(within(leche).getByRole('button', { name: 'Más' }));
    fireEvent.click(screen.getByRole('button', { name: 'Traspasar 1 artículo' }));
    expect(await screen.findByText('Ese traspaso ya se había hecho. Revisa las existencias antes de repetirlo.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Traspasar 1 artículo' }));
    await waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(2));
    expect(back.createTransfer.mock.calls[1][0].clientKey).not.toBe(back.createTransfer.mock.calls[0][0].clientKey);
  });

  // Otro origen u otro destino es otro traspaso: con la clave del borrador anterior el servidor lo
  // rechazaría como clave reusada.
  test('cambiar el origen o invertir renueva la clave', async () => {
    back.createTransfer.mockRejectedValueOnce(new Error('red'));
    montar();
    fireEvent.click(within(await fila('Leche entera')).getByRole('button', { name: 'Más' }));
    fireEvent.click(screen.getByRole('button', { name: 'Traspasar 1 artículo' }));
    await waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByRole('button', { name: 'Invertir' }));
    fireEvent.click(screen.getByRole('button', { name: 'Invertir' }));
    fireEvent.click(within(await fila('Leche entera')).getByRole('button', { name: 'Más' }));
    fireEvent.click(screen.getByRole('button', { name: 'Traspasar 1 artículo' }));
    await waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(2));
    expect(back.createTransfer.mock.calls[1][0].clientKey).not.toBe(back.createTransfer.mock.calls[0][0].clientKey);
  });
});
