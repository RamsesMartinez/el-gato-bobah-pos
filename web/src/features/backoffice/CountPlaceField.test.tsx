import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import { CountPlaceField } from './CountPlaceField';
import { useSessionStore } from '../../stores/session';

const back = vi.hoisted(() => ({ countPlaces: vi.fn(), createCountPlace: vi.fn(), setCountPlace: vi.fn() }));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
const toasts = vi.hoisted(() => ({ create: vi.fn() }));
vi.mock('../../components/ui/toaster', () => ({ toaster: toasts }));

beforeEach(() => {
  back.countPlaces.mockResolvedValue([
    { id: 1, name: 'Refri', position: 0, ingredients: 3 },
    { id: 2, name: 'Barra', position: 1, ingredients: 0 },
  ]);
  back.setCountPlace.mockResolvedValue(undefined);
  useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'admin', permissions: ['inventory.configure', 'inventory.count', 'inventory.view'] } as never });
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

function montar(placeId: number | null) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><CountPlaceField ingredientId={10} placeId={placeId} /></Provider></QueryClientProvider>);
}

// Spec 043: el lugar se llama «Ubicación» y su vacío «Sin definir»; crear una pide configurar el
// almacén, y quien solo cuenta elige entre las que hay.
describe('ubicación del artículo (spec 040, renombrada en la 043)', () => {
  test('el renglón dice la ubicación o «Sin definir» y no hay campo de texto libre', async () => {
    montar(1);
    expect(await screen.findByRole('button', { name: /Refri/ })).toBeInTheDocument();
    expect(screen.getByText('Ubicación')).toBeInTheDocument();
    expect(document.querySelector('input:not([type="hidden"])')).toBeNull();
    expect(document.querySelector('select')).toBeNull();
    cleanup();
    montar(null);
    expect(await screen.findByRole('button', { name: /Sin definir/ })).toBeInTheDocument();
  });

  test('una ubicación archivada (ya no está en la lista) se ve «Sin definir»', async () => {
    montar(99);
    expect(await screen.findByRole('button', { name: /Sin definir/ })).toBeInTheDocument();
  });

  test('sin ninguna ubicación en la empresa, el campo da el ejemplo', async () => {
    back.countPlaces.mockResolvedValue([]);
    montar(null);
    expect(await screen.findByRole('button', { name: /Ej\. Refri, Bodega, Barra/ })).toBeInTheDocument();
  });

  test('la hoja ofrece «Sin definir» y las ubicaciones en su orden, sin abrir el teclado; elegir lo guarda', async () => {
    montar(null);
    fireEvent.click(await screen.findByRole('button', { name: /Sin definir/ }));
    const barra = await screen.findByRole('button', { name: 'Barra' });
    const refri = screen.getByRole('button', { name: 'Refri' });
    expect(refri.compareDocumentPosition(barra) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(document.activeElement?.tagName).not.toBe('INPUT');
    fireEvent.click(screen.getByRole('button', { name: 'Barra' }));
    await waitFor(() => expect(back.setCountPlace).toHaveBeenCalledWith(10, 2));
    expect(await screen.findByRole('button', { name: /Barra/ })).toBeInTheDocument();
  });

  test('buscar no distingue mayúsculas ni acentos', async () => {
    montar(null);
    fireEvent.click(await screen.findByRole('button', { name: /Sin definir/ }));
    fireEvent.change(await screen.findByPlaceholderText('Buscar…'), { target: { value: 'BÁRRA' } });
    expect(await screen.findByRole('button', { name: 'Barra' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Refri' })).toBeNull();
  });

  // R4: con el teclado abierto en 600 px, «Crear» al final de la lista quedaba tapado.
  test('«Crear» va primero, debajo del buscador; si ya existía con otra mayúscula, lo dice y la elige', async () => {
    back.createCountPlace.mockRejectedValue(new ApiError(409, 'CONFLICT', 'esa ubicación ya existe', 'r', { existingId: 1 }));
    montar(null);
    fireEvent.click(await screen.findByRole('button', { name: /Sin definir/ }));
    fireEvent.change(await screen.findByPlaceholderText('Buscar…'), { target: { value: 'R' } });
    const crear = await screen.findByRole('button', { name: /Crear «R»/ });
    expect(crear.compareDocumentPosition(screen.getByRole('button', { name: 'Refri' })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    fireEvent.change(screen.getByPlaceholderText('Buscar…'), { target: { value: 'Refrí grande' } });
    fireEvent.click(await screen.findByRole('button', { name: /Crear/ }));
    await waitFor(() => expect(back.setCountPlace).toHaveBeenCalledWith(10, 1));
    expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: 'Ya tenías «Refri»; quedó elegida' }));
  });

  test('quien solo cuenta no ve «Crear»: se le pide a un administrador', async () => {
    useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'cajero', permissions: ['inventory.count'] } as never });
    montar(null);
    fireEvent.click(await screen.findByRole('button', { name: /Sin definir/ }));
    fireEvent.change(await screen.findByPlaceholderText('Buscar…'), { target: { value: 'Bodega nueva' } });
    expect(await screen.findByText('Pídele a un administrador que la cree.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Crear/ })).toBeNull();
  });

  test('un nombre que el servidor rechaza lo dice y no deja la hoja trabada; el tope es de 30', async () => {
    back.createCountPlace.mockRejectedValue(new ApiError(400, 'VALIDATION', 'largo', 'r'));
    montar(null);
    fireEvent.click(await screen.findByRole('button', { name: /Sin definir/ }));
    const buscar = await screen.findByPlaceholderText('Buscar…');
    expect(buscar).toHaveAttribute('maxlength', '30');
    fireEvent.change(buscar, { target: { value: 'Otro nombre' } });
    fireEvent.click(await screen.findByRole('button', { name: /Crear/ }));
    await waitFor(() => expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: 'La ubicación lleva de 1 a 30 letras' })));
    expect(back.setCountPlace).not.toHaveBeenCalled();
  });
});
