import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { RejillaDeToques } from './RejillaDeToques';
import { FECHA_DEL_LAYOUT } from './zonas-del-pos';
import type { Empresa } from './api';

// LA REJILLA, PINTADA (US1 de la 019).
//
// Lo que se prueba aquí es lo que ninguna barrera del backend puede atrapar: que la rejilla tenga
// la forma de la pantalla que dice describir, que el número se pueda LEER y no solo intuir por el
// tono, que con cero datos lo diga en vez de fingir, y que la leyenda salga con su fecha.

const EMPRESAS: Empresa[] = [
  { id: 1, slug: 'gatobobah', name: 'El Gato Bobah', activa: true, createdAt: '2026-01-01T00:00:00Z' },
];

function respuesta(cuerpo: unknown) {
  return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(cuerpo) } as Response);
}

function celdas(conteos: Record<number, number> = {}) {
  return Array.from({ length: 84 }, (_, i) => ({ celda: i, veces: conteos[i] ?? 0 }));
}

const CON_TOQUES = {
  pantalla: 'pos',
  orientacion: 'horizontal',
  rejilla: { columnas: 12, filas: 7 },
  periodo: { desde: '2026-09-06', hasta: '2026-09-12' },
  celdas: celdas({ 37: 412, 0: 5 }),
  porRol: [
    { rol: 'cajero', veces: 400 },
    { rol: null, veces: 17 },
  ],
};

const VACIA = { ...CON_TOQUES, celdas: celdas(), porRol: [] };

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('la rejilla de toques', () => {
  it('pinta las 84 celdas con la forma de la pantalla', async () => {
    vi.stubGlobal('fetch', vi.fn(() => respuesta(CON_TOQUES)));
    render(<RejillaDeToques empresas={EMPRESAS} />);

    const zonas = await screen.findAllByRole('gridcell');
    // Las de cero también se pintan: un hueco donde nadie tocó se lee como un defecto de la
    // pantalla, y «qué parte no toca nadie» es la mitad de la pregunta.
    expect(zonas).toHaveLength(84);
  });

  it('el número va ESCRITO dentro de la celda, no solo en el tono', async () => {
    // Una escala de color sola es ilegible para quien no distingue esos tonos, y obliga a comparar
    // a ojo cuánto es «más oscuro».
    vi.stubGlobal('fetch', vi.fn(() => respuesta(CON_TOQUES)));
    render(<RejillaDeToques empresas={EMPRESAS} />);

    expect(await screen.findByText('412')).toBeTruthy();
    expect(screen.getByText('5')).toBeTruthy();
  });

  it('cada celda dice dónde está, para quien no ve la rejilla', async () => {
    vi.stubGlobal('fetch', vi.fn(() => respuesta(CON_TOQUES)));
    render(<RejillaDeToques empresas={EMPRESAS} />);

    // La celda 37 en una rejilla de 12 columnas es fila 3, columna 1.
    const zona = await screen.findByLabelText(/fila 3.*columna 1.*412/i);
    expect(zona).toBeTruthy();
  });

  it('con cero toques lo dice, en vez de pintar una rejilla vacía', async () => {
    vi.stubGlobal('fetch', vi.fn(() => respuesta(VACIA)));
    render(<RejillaDeToques empresas={EMPRESAS} />);

    expect(await screen.findByText(/todavía no hay toques/i)).toBeTruthy();
  });

  it('la leyenda sale con la fecha del layout que describe', async () => {
    // Sin la fecha, quien mire datos de hace tres meses creería que la referencia sigue vigente y
    // movería el botón equivocado.
    vi.stubGlobal('fetch', vi.fn(() => respuesta(CON_TOQUES)));
    render(<RejillaDeToques empresas={EMPRESAS} />);

    await screen.findAllByRole('gridcell');
    expect(screen.getByText(new RegExp(FECHA_DEL_LAYOUT))).toBeTruthy();
    expect(screen.getByText(/los productos/i)).toBeTruthy();
  });

  it('al cambiar de orientación pide la otra, y NO las suma', async () => {
    const pedidas: string[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        pedidas.push(url);
        return respuesta(url.includes('vertical') ? { ...CON_TOQUES, orientacion: 'vertical', rejilla: { columnas: 7, filas: 12 } } : CON_TOQUES);
      }),
    );
    render(<RejillaDeToques empresas={EMPRESAS} />);
    await screen.findAllByRole('gridcell');

    await userEvent.click(screen.getByRole('button', { name: /vertical/i }));

    await waitFor(() => {
      expect(pedidas.some((u) => u.includes('orientacion=vertical'))).toBe(true);
    });
    // Cada petición pide UNA orientación: la celda 37 es otro lugar en cada forma, y mezclarlas
    // pintaría un mapa que nadie tocó nunca.
    for (const u of pedidas) {
      expect(u).toMatch(/orientacion=(horizontal|vertical)/);
    }
  });
});
