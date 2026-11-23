import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import { WarehousesSettings } from './WarehousesSettings';

const back = vi.hoisted(() => ({
  warehouses: vi.fn(), createWarehouse: vi.fn(), renameWarehouse: vi.fn(), archiveWarehouse: vi.fn(), restoreWarehouse: vi.fn(), setSalesWarehouse: vi.fn(),
}));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

beforeEach(() => {
  back.warehouses.mockResolvedValue({ items: [
    { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 16 },
    { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 0 },
  ] });
  for (const f of [back.createWarehouse, back.renameWarehouse, back.archiveWarehouse, back.restoreWarehouse, back.setSalesWarehouse]) f.mockResolvedValue(undefined);
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><MemoryRouter initialEntries={['/negocio']}><WarehousesSettings /></MemoryRouter></Provider></QueryClientProvider>);
}

describe('almacenes de la sucursal (spec 094, maqueta A3)', () => {
  // «Lugares» al lado confundía (prueba con usuario nuevo): el texto no usa esa palabra.
  test('separa donde se vende de donde se guarda, con lo que tiene cada uno', async () => {
    montar();
    expect(await screen.findByText('Dónde guardas mercancía: tu local y, si tienes, una bodega aparte.')).toBeInTheDocument();
    const venta = await screen.findByRole('group', { name: 'Donde se vende' });
    expect(within(venta).getByText('Local')).toBeInTheDocument();
    expect(within(venta).getByText('De venta')).toBeInTheDocument();
    expect(within(venta).getByText(/16 artículos con existencia/)).toBeInTheDocument();
    const guarda = screen.getByRole('group', { name: 'Donde se guarda' });
    expect(within(guarda).getByText('Bodega')).toBeInTheDocument();
    expect(within(guarda).getByText('Vacío')).toBeInTheDocument();
  });

  // La lista puede llegar sin `items` (un slice nil del servidor sale como null): no truena.
  test('sin almacenes que listar no se cae', async () => {
    back.warehouses.mockResolvedValue({});
    montar();
    expect(await screen.findByRole('button', { name: 'Agregar almacén' })).toBeInTheDocument();
  });

  test('agrega un almacén con una sugerencia', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Agregar almacén' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Cuarto frío' }));
    fireEvent.click(screen.getByRole('button', { name: 'Agregar' }));
    await waitFor(() => expect(back.createWarehouse).toHaveBeenCalledWith('Cuarto frío'));
  });

  test('un nombre repetido lo dice en la hoja', async () => {
    back.createWarehouse.mockRejectedValue(new ApiError(409, 'WAREHOUSE_NAME_TAKEN', 'ya hay un almacén con ese nombre', ''));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Agregar almacén' }));
    expect(screen.getByLabelText('Nombre del almacén')).toHaveAttribute('placeholder', 'Nombre del almacén');
    fireEvent.change(screen.getByLabelText('Nombre del almacén'), { target: { value: 'bodega' } });
    fireEvent.click(screen.getByRole('button', { name: 'Agregar' }));
    expect(await screen.findByText('Ya hay un almacén con ese nombre')).toBeInTheDocument();
  });

  // Archivar pide confirmar: el botón y el «Sí» no son el mismo toque.
  test('archivar un vacío pide confirmar', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Bodega/ }));
    fireEvent.click(await screen.findByRole('button', { name: 'Archivar' }));
    expect(back.archiveWarehouse).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Sí, archivar' }));
    await waitFor(() => expect(back.archiveWarehouse).toHaveBeenCalledWith(2));
  });

  test('con compras por recibir dice por qué no se archiva', async () => {
    back.archiveWarehouse.mockRejectedValue(new ApiError(409, 'WAREHOUSE_PENDING_PURCHASES', 'x', ''));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Bodega/ }));
    fireEvent.click(await screen.findByRole('button', { name: 'Archivar' }));
    fireEvent.click(screen.getByRole('button', { name: 'Sí, archivar' }));
    expect(await screen.findByText('Tiene compras por recibir: recíbelas o márcalas como que no llegan primero')).toBeInTheDocument();
  });

  // La persona nueva no encontró cómo cambiar el almacén de venta (prueba con usuario nuevo, 094).
  test('otro almacén se puede volver el de venta, con confirmación', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Bodega/ }));
    fireEvent.click(await screen.findByRole('button', { name: 'Vender desde aquí' }));
    expect(back.setSalesWarehouse).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Sí, vender desde Bodega' }));
    await waitFor(() => expect(back.setSalesWarehouse).toHaveBeenCalledWith(2));
  });

  test('el de venta y uno con existencias no ofrecen archivar, y dicen por qué', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /^Local/ }));
    expect(await screen.findByText('De aquí salen las ventas. Este almacén no se archiva.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Archivar' })).toBeNull();
  });
});
