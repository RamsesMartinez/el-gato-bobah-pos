import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../../components/ui/provider';
import { ApiError } from '../../../api/client';
import { BringSheet } from './BringSheet';

const back = vi.hoisted(() => ({ createTransfer: vi.fn() }));
vi.mock('../../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
vi.mock('../../../components/ui/toaster', () => ({ toaster: { create: vi.fn() } }));

const BODEGA = { warehouseId: 2, name: 'Bodega', onHand: '3000' };
const COCINA = { warehouseId: 3, name: 'Cocina', onHand: '9000' };

beforeEach(() => back.createTransfer.mockResolvedValue({ id: 99 }));
afterEach(() => { cleanup(); vi.clearAllMocks(); });

function montar(onHand: string, sources = [BODEGA], unit = 'g') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider>
    <BringSheet item={{ itemType: 'ingrediente', id: 8, name: 'Azúcar', unit, onHand }} sources={sources}
      to={{ id: 1, name: 'Local' }} onClose={() => {}} />
  </Provider></QueryClientProvider>);
}

describe('«Traer de…» desde Existencias (spec 094)', () => {
  // Faltan 4 g: en kilos son 0.004, y redondear a dos decimales proponía 0 y dejaba el botón apagado.
  test('una falta chica no se propone como cero', async () => {
    montar('-4');
    expect(await screen.findByLabelText('Cuánto traer')).toHaveTextContent('0.004 kg');
    fireEvent.click(screen.getByRole('button', { name: /^Traspasar/ }));
    await waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(1));
    expect(back.createTransfer.mock.calls[0][0].lines[0].qty).toBe('4');
  });

  // Como en Traspaso (cierre del #65): con 105 g en Bodega se ve «105 g», y la cantidad iba en kg.
  test('la cantidad va en la unidad en que se ve la existencia del origen', async () => {
    montar('-4', [{ warehouseId: 2, name: 'Bodega', onHand: '105' }]);
    expect(await screen.findByLabelText('Cuánto traer')).toHaveTextContent('4 g');
    fireEvent.click(screen.getByRole('button', { name: /^Traspasar 4 g/ }));
    await waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(1));
    expect(back.createTransfer.mock.calls[0][0].lines[0].qty).toBe('4');
  });

  // Con varios almacenes que tienen el artículo, propone el que más tiene, lo dice y deja cambiarlo.
  test('con varios orígenes propone el que más tiene y deja elegir otro', async () => {
    montar('-2000', [BODEGA, COCINA]);
    expect(await screen.findByText(/Cocina es el que más tiene/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Cocina/ }));
    fireEvent.click(await screen.findByRole('button', { name: /^Bodega/ }));
    expect(await screen.findByText(/En Bodega hay/)).toBeInTheDocument();
    fireEvent.click(await screen.findByRole('button', { name: /^Traspasar/ }));
    await waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(1));
    expect(back.createTransfer.mock.calls[0][0].fromWarehouseId).toBe(2);
  });

  test('dejar el origen en negativo pide confirmar y reenvía con la misma clave', async () => {
    back.createTransfer.mockRejectedValueOnce(new ApiError(409, 'NEEDS_CONFIRM', 'x', '',
      { lines: [{ itemType: 'ingrediente', id: 8, available: '1000' }] }));
    montar('-2000');
    fireEvent.click(await screen.findByRole('button', { name: /^Traspasar/ }));
    expect(await screen.findByText(/Bodega se quedaría en negativo/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Traspasar de todos modos' }));
    await waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(2));
    const [first, second] = back.createTransfer.mock.calls.map((c) => c[0]);
    expect(second.confirmNegative).toBe(true);
    expect(second.clientKey).toBe(first.clientKey);
  });

  test('una clave ya usada se renueva y avisa que el traspaso ya se había hecho', async () => {
    back.createTransfer.mockRejectedValueOnce(new ApiError(409, 'TRANSFER_KEY_REUSED', 'x', ''));
    montar('-2000');
    fireEvent.click(await screen.findByRole('button', { name: /^Traspasar/ }));
    expect(await screen.findByText('Ese traspaso ya se había hecho. Revisa las existencias antes de repetirlo.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /^Traspasar/ }));
    await waitFor(() => expect(back.createTransfer).toHaveBeenCalledTimes(2));
    expect(back.createTransfer.mock.calls[1][0].clientKey).not.toBe(back.createTransfer.mock.calls[0][0].clientKey);
  });
});
