import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../../components/ui/provider';
import { useSessionStore } from '../../../stores/session';
import { ArticlePlaceField } from './ArticlePlaceField';

const back = vi.hoisted(() => ({ countPlaces: vi.fn(), createCountPlace: vi.fn(), warehouses: vi.fn() }));
vi.mock('../../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

const dos = { items: [{ id: 1, name: 'Local', isSales: true, isActive: true }, { id: 2, name: 'Bodega', isSales: false, isActive: true }] };

beforeEach(() => {
  useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'admin', permissions: ['inventory.view', 'inventory.configure'] } as never });
  back.warehouses.mockResolvedValue(dos);
  back.countPlaces.mockImplementation((w?: number) => Promise.resolve(w === 2
    ? [{ id: 20, name: 'Anaquel', position: 0, ingredients: 1 }]
    : [{ id: 10, name: 'Refri', position: 0, ingredients: 3 }, { id: 11, name: 'Barra', position: 1, ingredients: 0 }]));
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

function montar(value: string, onChange = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider>
    <ArticlePlaceField name="Leche" value={value} onChange={onChange} />
  </Provider></QueryClientProvider>);
  return onChange;
}

// El artículo tiene UN lugar para contar, de cualquier almacén (cierre del #65). La ficha no decía de
// qué almacén era el lugar, y elegir uno de otro almacén lo sacaba del recorrido del primero sin avisar.
describe('Ubicación en la ficha con varios almacenes', () => {
  test('dice de qué almacén es el lugar elegido', async () => {
    montar('20');
    expect(await screen.findByText('Se cuenta en Bodega')).toBeInTheDocument();
  });

  test('con un solo almacén no dice almacén', async () => {
    back.warehouses.mockResolvedValue({ items: [dos.items[0]] });
    montar('10');
    expect(await screen.findByRole('button', { name: /Refri/ })).toBeInTheDocument();
    await waitFor(() => expect(back.warehouses).toHaveBeenCalled());
    expect(screen.queryByText(/Se cuenta en/)).toBeNull();
  });

  test('elegir un lugar de otro almacén pide confirmar que cambia de recorrido; volver no cambia nada', async () => {
    const onChange = montar('10');
    await screen.findByText('Se cuenta en Local');
    fireEvent.click(screen.getByRole('button', { name: /Refri/ }));
    fireEvent.click(await screen.findByRole('button', { name: /Anaquel/ }));
    expect(await screen.findByText(/pasa al recorrido del conteo de Bodega y sale del de Local/)).toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Volver' }));
    await waitFor(() => expect(screen.queryByText(/pasa al recorrido/)).toBeNull());
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: /Refri/ }));
    fireEvent.click(await screen.findByRole('button', { name: /Anaquel/ }));
    fireEvent.click(await screen.findByRole('button', { name: 'Cambiar de almacén' }));
    await waitFor(() => expect(onChange).toHaveBeenCalledWith('20'));
  });

  test('un lugar del mismo almacén se guarda sin preguntar', async () => {
    const onChange = montar('10');
    await screen.findByText('Se cuenta en Local');
    fireEvent.click(screen.getByRole('button', { name: /Refri/ }));
    fireEvent.click(await screen.findByRole('button', { name: /Barra/ }));
    await waitFor(() => expect(onChange).toHaveBeenCalledWith('11'));
  });
});
