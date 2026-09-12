import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MapaDeUso } from './MapaDeUso';
import type { Empresa } from './api';

// EL MAPA, PINTADO (US1). Lo que se prueba es lo que una barrera del backend no puede atrapar: que
// la pantalla diga lo que el servidor manda, y que cuando no hay nada lo diga en vez de fingir.

const EMPRESAS: Empresa[] = [
  { id: 1, slug: 'gatobobah', name: 'El Gato Bobah', activa: true, createdAt: '2026-01-01T00:00:00Z' },
];

function respuesta(cuerpo: unknown) {
  return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(cuerpo) } as Response);
}

const CON_USO = {
  periodo: { desde: '2026-09-06', hasta: '2026-09-12' },
  pantallas: [
    {
      pantalla: 'pos',
      aperturas: 40,
      acciones: [{ accion: 'cobrar', veces: 12 }],
      porRol: [
        { rol: 'cajero', veces: 50 },
        { rol: null, veces: 2 },
      ],
    },
    { pantalla: 'gastos', aperturas: 0, acciones: [], porRol: [] },
  ],
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('el mapa de uso', () => {
  it('pinta las pantallas con su número, y el número va escrito', async () => {
    vi.stubGlobal('fetch', vi.fn(() => respuesta(CON_USO)));
    render(<MapaDeUso empresas={EMPRESAS} />);

    expect(await screen.findByRole('button', { name: /Punto de venta/ })).toBeInTheDocument();
    // El número dentro de la celda: una escala de color sola obliga a comparar tonos a ojo, y es
    // ilegible para quien no los distingue.
    expect(screen.getByText('52')).toBeInTheDocument(); // 40 aperturas + 12 cobros
  });

  it('la pantalla que nadie abrió aparece en cero, no se omite', async () => {
    vi.stubGlobal('fetch', vi.fn(() => respuesta(CON_USO)));
    render(<MapaDeUso empresas={EMPRESAS} />);

    // «Qué no usa nadie» es la mitad de la pregunta: una pantalla que se omite por no tener filas
    // se lee como que no existe.
    expect(await screen.findByText('Gastos')).toBeInTheDocument();
    expect(screen.getByText('0')).toBeInTheDocument();
  });

  it('nombra «sin corte» en vez de dejar un hueco', async () => {
    vi.stubGlobal('fetch', vi.fn(() => respuesta(CON_USO)));
    render(<MapaDeUso empresas={EMPRESAS} />);

    expect(await screen.findByText(/sin corte 2/)).toBeInTheDocument();
  });

  it('despliega las acciones de una pantalla', async () => {
    vi.stubGlobal('fetch', vi.fn(() => respuesta(CON_USO)));
    render(<MapaDeUso empresas={EMPRESAS} />);

    await userEvent.click(await screen.findByRole('button', { name: /Punto de venta/ }));
    expect(screen.getByText(/Cobrar/)).toBeInTheDocument();
    expect(screen.getByText('12')).toBeInTheDocument();
  });

  it('con cero uso lo dice, y no pinta un mapa vacío (FR-015)', async () => {
    vi.stubGlobal('fetch', vi.fn(() => respuesta({
      periodo: { desde: '2026-09-06', hasta: '2026-09-12' },
      pantallas: [{ pantalla: 'pos', aperturas: 0, acciones: [], porRol: [] }],
    })));
    render(<MapaDeUso empresas={EMPRESAS} />);

    expect(await screen.findByText(/Todavía no hay uso registrado/)).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it('cambiar el periodo vuelve a pedir los datos', async () => {
    const fetchStub = vi.fn(() => respuesta(CON_USO));
    vi.stubGlobal('fetch', fetchStub);
    render(<MapaDeUso empresas={EMPRESAS} />);
    await screen.findByRole('button', { name: /Punto de venta/ });

    await userEvent.click(screen.getByRole('button', { name: '30 días' }));
    expect(fetchStub.mock.calls.length).toBeGreaterThan(1);
    const segunda = fetchStub.mock.calls[1] as unknown as [string];
    expect(String(segunda[0])).toContain('desde=');
  });
});
