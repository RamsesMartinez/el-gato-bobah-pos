import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { OptionPriceFields } from './OptionPriceFields';
import type { DesglosePrecio } from './precioPlataforma';

const api = vi.hoisted(() => ({
  setPlatformOptionPrice: vi.fn(() => Promise.resolve({})),
  removePlatformOptionPrice: vi.fn(() => Promise.resolve()),
}));
vi.mock('../../api/pos', () => ({ posApi: api }));

function montar(desglose: DesglosePrecio) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <OptionPriceFields
          optionId={300} optionName="Queso extra" plataforma="Uber Eats" plataformaId={5}
          desglose={desglose} onDone={() => {}}
        />
      </Provider>
    </QueryClientProvider>,
  );
}

const calculado: DesglosePrecio = { base: 20, calculado: 27, vigente: 27, esManual: false };
const manual: DesglosePrecio = { base: 20, calculado: 27, vigente: 30, esManual: true };

describe('OptionPriceFields', () => {
  beforeEach(() => vi.clearAllMocks());

  // El operador está corrigiendo un número que el sistema calculó. Sin ver de dónde salió, corrige
  // a ciegas y no tiene forma de saber si 30 es mucho o poco.
  it('muestra de dónde sale el cargo vigente', () => {
    montar(calculado);
    expect(screen.getByText('Cargo de mostrador')).toBeInTheDocument();
    expect(screen.getByText('Cargo calculado')).toBeInTheDocument();
    expect(screen.getByDisplayValue('27')).toBeInTheDocument();
  });

  it('guarda el cargo capturado', async () => {
    montar(calculado);
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '33.5' } });
    fireEvent.click(screen.getByRole('button', { name: /guardar cargo/i }));
    await waitFor(() => expect(api.setPlatformOptionPrice).toHaveBeenCalledWith(300, 5, 33.5));
  });

  // Un extra sin costo ("sin cebolla") es normal y su cargo es 0: si el botón se bloqueara en 0, no
  // habría forma de capturarlo desde la pantalla.
  it('acepta 0 y rechaza negativo', () => {
    montar(calculado);
    const campo = screen.getByRole('spinbutton');
    const boton = screen.getByRole('button', { name: /guardar cargo/i });

    fireEvent.change(campo, { target: { value: '0' } });
    expect(boton).not.toBeDisabled();

    fireEvent.change(campo, { target: { value: '-1' } });
    expect(boton).toBeDisabled();
  });

  // Quitar solo aparece si hay algo que quitar: un botón que no hace nada enseña que los botones a
  // veces no hacen nada.
  it('el botón de quitar solo sale con un cargo capturado', () => {
    const { unmount } = montar(calculado);
    expect(screen.queryByRole('button', { name: /quitar cargo/i })).not.toBeInTheDocument();
    unmount();

    montar(manual);
    expect(screen.getByRole('button', { name: /quitar cargo/i })).toBeInTheDocument();
  });

  it('quitar el cargo llama al backend con la opción y la plataforma', async () => {
    montar(manual);
    fireEvent.click(screen.getByRole('button', { name: /quitar cargo/i }));
    await waitFor(() => expect(api.removePlatformOptionPrice).toHaveBeenCalledWith(300, 5));
  });
});
