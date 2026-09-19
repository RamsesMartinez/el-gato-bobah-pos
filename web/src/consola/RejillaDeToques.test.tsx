import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { RejillaDeToques } from './RejillaDeToques';
import * as api from './api';

vi.mock('./api', async () => {
  const real = await vi.importActual<typeof api>('./api');
  return { ...real, rejillaDeToques: vi.fn() };
});

// REGRESIÓN de un defecto real (encontrado el 2026-09-15).
//
// `rejilla?.rejilla.columnas` cortaba la cadena opcional un nivel antes: una respuesta que trae
// `celdas` pero no `rejilla` pasa la primera guarda y revienta en la segunda. La consola NO tiene
// frontera de error, así que eso no deja la rejilla vacía — tumba la pantalla entera.
//
// Se manifestaba como un test intermitente de la consola, no como un error reproducible, y por eso
// llevaba semanas escondido. El comentario que está justo encima de esa línea decía exactamente lo
// que había que evitar.
describe('RejillaDeToques con una respuesta a la que le falta la forma', () => {
  beforeEach(() => {
    // Lo que llega cuando responde una versión vieja de la API, o un proxy que devuelve otra cosa.
    vi.mocked(api.rejillaDeToques).mockResolvedValue({
      celdas: [{ celda: 0, veces: 3 }],
    } as never);
  });

  it('muestra la rejilla vacía en vez de tumbar la consola', async () => {
    render(<RejillaDeToques empresas={[]} />);
    // La respuesta llega DESPUÉS del primer render: el defecto solo aparece al reintentar con el
    // estado ya puesto, que es por lo que una aserción síncrona no lo veía.
    await waitFor(() => expect(api.rejillaDeToques).toHaveBeenCalled());
    expect(await screen.findByText(/Qué hay en cada zona/)).toBeInTheDocument();
  });
});
