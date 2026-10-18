import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { MemoryRouter, Route, Routes } from 'react-router';
import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import { CountPlacesSettings } from './CountPlacesSettings';

const back = vi.hoisted(() => ({
  countPlaces: vi.fn(), createCountPlace: vi.fn(), renameCountPlace: vi.fn(), moveCountPlace: vi.fn(), archiveCountPlace: vi.fn(),
}));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

beforeEach(() => {
  back.countPlaces.mockResolvedValue([
    { id: 1, name: 'Refri', position: 0, ingredients: 12 },
    { id: 2, name: 'Barra', position: 1, ingredients: 0 },
    { id: 3, name: 'Bodega', position: 2, ingredients: 4 },
  ]);
  for (const f of [back.createCountPlace, back.renameCountPlace, back.moveCountPlace, back.archiveCountPlace]) f.mockResolvedValue(undefined);
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

function montar(hash = '') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><MemoryRouter initialEntries={[`/negocio${hash}`]}>
    <Routes><Route path="/negocio" element={<CountPlacesSettings />} /><Route path="/almacen" element={<p>Conteo abierto aquí</p>} /></Routes>
  </MemoryRouter></Provider></QueryClientProvider>);
}
const row = async (name: string) => (await screen.findByText(name, { exact: true })).closest('[data-place]') as HTMLElement;

describe('lugares del recorrido (spec 040, maqueta C)', () => {
  // La tercera prueba sin contexto llegó desde el conteo y no supo cómo volver.
  test('si se llegó desde el conteo, ofrece volver a él', async () => {
    montar('#lugares');
    fireEvent.click(await screen.findByRole('button', { name: '‹ Volver al conteo' }));
    expect(await screen.findByText('Conteo abierto aquí')).toBeInTheDocument();
    cleanup();
    montar();
    await screen.findByText('Lugares');
    expect(screen.queryByRole('button', { name: '‹ Volver al conteo' })).toBeNull();
  });

  test('cada renglón en una línea con su agarradera; «Agregar lugar» junto al título', async () => {
    montar();
    expect(within(await row('Refri')).getByText('12 artículos')).toBeInTheDocument();
    expect(within(await row('Barra')).getByText('Sin artículos')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Agregar lugar' })).toBeInTheDocument();
    expect(screen.getByText('Mantén presionada ≡ para mover un lugar')).toBeInTheDocument();
    const grip = within(await row('Refri')).getByRole('button', { name: 'Mover Refri' });
    expect(grip).toHaveStyle({ touchAction: 'none' });
  });

  // jsdom no mide renglones (todo mide 0 px), así que el arrastre y las flechas no se pueden simular
  // aquí: los cubre la prueba táctil de Playwright a 1024×600 (T018). Aquí solo que la agarradera
  // se ofrece al teclado y a un lector de pantalla.
  test('la agarradera se puede usar con el teclado', async () => {
    montar();
    const grip = within(await row('Bodega')).getByRole('button', { name: 'Mover Bodega' });
    expect(grip).toHaveAttribute('aria-roledescription', 'sortable');
    expect(grip).toHaveAttribute('tabindex', '0');
  });

  test('renombrar desde Editar; un nombre repetido lo dice', async () => {
    back.renameCountPlace.mockRejectedValue(new ApiError(409, 'CONFLICT', 'ese lugar ya existe', 'r', { existingId: 2 }));
    montar();
    fireEvent.click(within(await row('Refri')).getByRole('button', { name: 'Editar' }));
    fireEvent.change(await screen.findByLabelText('Nombre del lugar'), { target: { value: 'barra' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    expect(await screen.findByText(/ya existe/)).toBeInTheDocument();
  });

  test('archivar sin artículos pide confirmar; con artículos pregunta a dónde pasan', async () => {
    montar();
    fireEvent.click(within(await row('Barra')).getByRole('button', { name: 'Editar' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Archivar' }));
    expect(await screen.findByText('Se archiva «Barra». Ya no aparece en las listas.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Sí, archivar' }));
    await waitFor(() => expect(back.archiveCountPlace).toHaveBeenCalledWith(2, undefined));
    cleanup();

    montar();
    fireEvent.click(within(await row('Refri')).getByRole('button', { name: 'Editar' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Archivar' }));
    expect(await screen.findByText('Refri tiene 12 artículos. ¿A qué lugar pasan?')).toBeInTheDocument();
    const pasar = screen.getByRole('button', { name: 'Pasar y archivar' });
    expect(pasar).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'Sin lugar' }));
    expect(screen.getByRole('button', { name: 'Sin lugar', pressed: true })).toBeInTheDocument();
    fireEvent.click(pasar);
    await waitFor(() => expect(back.archiveCountPlace).toHaveBeenCalledWith(1, null));
  });

  test('Archivar va apartado de Guardar', async () => {
    montar();
    fireEvent.click(within(await row('Refri')).getByRole('button', { name: 'Editar' }));
    const guardar = await screen.findByRole('button', { name: 'Guardar' });
    const archivar = screen.getByRole('button', { name: 'Archivar' });
    expect(guardar.parentElement).toHaveStyle({ justifyContent: 'space-between' });
    expect(archivar.parentElement).toBe(guardar.parentElement);
  });
});
