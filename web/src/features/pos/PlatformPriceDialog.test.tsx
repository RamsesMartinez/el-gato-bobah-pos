import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { PlatformPriceDialog } from './PlatformPriceDialog';
import { desglosePrecio, type DesglosePrecio } from './precioPlataforma';
import type { Menu } from '../../types/pos';

const api = vi.hoisted(() => ({
  setPlatformPrice: vi.fn(() => Promise.resolve({})),
  removePlatformPrice: vi.fn(() => Promise.resolve()),
}));
vi.mock('../../api/pos', () => ({ posApi: api }));

function montar(desglose: DesglosePrecio) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <PlatformPriceDialog
          productId={77} productName="Boneless" plataforma="Uber Eats" plataformaId={5}
          desglose={desglose} isOpen onClose={() => {}}
        />
      </Provider>
    </QueryClientProvider>,
  );
}

const calculado: DesglosePrecio = { base: 434.98, calculado: 587.22, vigente: 587.22, esManual: false };
const manual: DesglosePrecio = { base: 100, calculado: 135, vigente: 149, esManual: true };

describe('PlatformPriceDialog', () => {
  beforeEach(() => vi.clearAllMocks());

  it('muestra de dónde sale el precio vigente y con qué lista se está cobrando', () => {
    montar(calculado);
    expect(screen.getByText(/Boneless en Uber Eats/)).toBeInTheDocument();
    expect(screen.getByText('Precio de mostrador')).toBeInTheDocument();
    expect(screen.getByText('Precio calculado')).toBeInTheDocument();
    expect(screen.getByDisplayValue('587.22')).toBeInTheDocument();
  });

  it('guarda el precio capturado', async () => {
    montar(calculado);
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '599' } });
    fireEvent.click(screen.getByRole('button', { name: /^guardar$/i }));
    await waitFor(() => expect(api.setPlatformPrice).toHaveBeenCalledWith(77, 5, 599));
  });

  // Un producto en $0 es SIEMPRE un error de captura, a diferencia de un extra sin costo. El check
  // de la tabla lo rechaza y aquí se evita el viaje.
  it('no deja guardar 0 ni negativo', () => {
    montar(calculado);
    const campo = screen.getByRole('spinbutton');
    const boton = screen.getByRole('button', { name: /^guardar$/i });

    fireEvent.change(campo, { target: { value: '0' } });
    expect(boton).toBeDisabled();

    fireEvent.change(campo, { target: { value: '-5' } });
    expect(boton).toBeDisabled();
  });

  it('quitar solo aparece con un precio capturado, y dice a cuánto vuelve', async () => {
    const { unmount } = montar(calculado);
    expect(screen.queryByRole('button', { name: /quitar precio/i })).not.toBeInTheDocument();
    unmount();

    montar(manual);
    expect(screen.getByText(/vuelva a/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /quitar precio/i }));
    await waitFor(() => expect(api.removePlatformPrice).toHaveBeenCalledWith(77, 5));
  });
});

// El diálogo no existe en mostrador, y el que lo decide es el desglose: en mostrador devuelve null y
// la pantalla no tiene qué pasarle. Es la barrera real — el precio base se edita en el catálogo, y
// confundir las dos listas es justo el error que esta feature no puede permitirse.
describe('en mostrador no hay diálogo que abrir', () => {
  const menu = {
    platforms: [{ id: 5, name: 'Uber Eats', markupPct: '35' }],
    platformPrices: {},
    platformModPrices: {},
  } as unknown as Menu;

  it('desglosePrecio en mostrador devuelve null', () => {
    expect(desglosePrecio(menu, null, 77, 100)).toBeNull();
    expect(desglosePrecio(menu, 5, 77, 100)).not.toBeNull();
  });
});
