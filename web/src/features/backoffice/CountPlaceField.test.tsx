import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import { CountPlaceField } from './CountPlaceField';

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
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

function montar(placeId: number | null) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><CountPlaceField ingredientId={10} placeId={placeId} /></Provider></QueryClientProvider>);
}

describe('lugar del artículo (spec 040, maqueta A)', () => {
  test('el renglón dice el lugar o «Sin lugar» y no hay campo de texto libre', async () => {
    montar(1);
    expect(await screen.findByRole('button', { name: /Refri/ })).toBeInTheDocument();
    expect(screen.getByText('Lugar')).toBeInTheDocument();
    expect(document.querySelector('input:not([type="hidden"])')).toBeNull();
    expect(document.querySelector('select')).toBeNull();
    cleanup();
    montar(null);
    expect(await screen.findByRole('button', { name: /Sin lugar/ })).toBeInTheDocument();
  });

  test('la hoja ofrece «Sin lugar» y los lugares en su orden, sin abrir el teclado; elegir lo guarda', async () => {
    montar(null);
    fireEvent.click(await screen.findByRole('button', { name: /Sin lugar/ }));
    const barra = await screen.findByRole('button', { name: 'Barra' });
    const refri = screen.getByRole('button', { name: 'Refri' });
    expect(refri.compareDocumentPosition(barra) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(document.activeElement?.tagName).not.toBe('INPUT');
    fireEvent.click(screen.getByRole('button', { name: 'Barra' }));
    await waitFor(() => expect(back.setCountPlace).toHaveBeenCalledWith(10, 2));
    // La hoja del artículo guarda una copia del lugar al abrirse: el botón tiene que decir el elegido
    // aunque esa copia no cambie (segunda prueba sin contexto, T022).
    expect(await screen.findByRole('button', { name: /Barra/ })).toBeInTheDocument();
  });

  test('un lugar nuevo se crea escribiéndolo; si ya existía con otra mayúscula, lo dice y lo elige', async () => {
    back.createCountPlace.mockRejectedValue(new ApiError(409, 'CONFLICT', 'ese lugar ya existe', 'r', { existingId: 1 }));
    montar(null);
    fireEvent.click(await screen.findByRole('button', { name: /Sin lugar/ }));
    fireEvent.change(await screen.findByPlaceholderText('Buscar…'), { target: { value: 'refri ' } });
    // «refri » ya coincide con «Refri» en la lista: escribir uno nuevo de verdad.
    fireEvent.change(screen.getByPlaceholderText('Buscar…'), { target: { value: 'Refrí grande' } });
    fireEvent.click(await screen.findByRole('button', { name: /Crear/ }));
    await waitFor(() => expect(back.setCountPlace).toHaveBeenCalledWith(10, 1));
    expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: 'Ya tenías «Refri»; quedó elegido' }));
  });

  test('un nombre que el servidor rechaza lo dice y no deja la hoja trabada', async () => {
    back.createCountPlace.mockRejectedValue(new ApiError(400, 'VALIDATION', 'largo', 'r'));
    montar(null);
    fireEvent.click(await screen.findByRole('button', { name: /Sin lugar/ }));
    const buscar = await screen.findByPlaceholderText('Buscar…');
    expect(buscar).toHaveAttribute('maxlength', '30');
    fireEvent.change(buscar, { target: { value: 'Otro nombre' } });
    fireEvent.click(await screen.findByRole('button', { name: /Crear/ }));
    await waitFor(() => expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: 'El lugar lleva de 1 a 30 letras' })));
    expect(back.setCountPlace).not.toHaveBeenCalled();
  });
});
