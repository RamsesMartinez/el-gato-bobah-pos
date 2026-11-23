import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { Provider } from '../../../components/ui/provider';
import { Toaster } from '../../../components/ui/toaster';
import { ApiError } from '../../../api/client';
import { VentaEnWebSection } from './VentaEnWebSection';

const ajustes = {
  businessName: 'Fonda Doña Lupe',
  name: 'fonda-dona-lupe', url: 'https://menu.test/fonda-dona-lupe', qrUrl: 'https://menu.test/q/0123456789ab',
  palette: 'brand', previousNames: ['lupita'],
};
const update = vi.fn();
const qrSvg = vi.fn(async (_: string) => '<svg data-testid="qr-svg"></svg>');

vi.mock('../../../api/admin', () => ({
  adminApi: {
    webSalesSummary: () => Promise.resolve({ onWeb: 142, withWebPrice: 3, total: 160 }),
    publicMenu: () => Promise.resolve(ajustes),
    updatePublicMenu: (b: unknown) => update(b),
  },
}));
vi.mock('./menuPublico/qr', () => ({
  qrSvg: (u: string) => qrSvg(u),
  descargarPng: vi.fn(), descargarSvg: vi.fn(), imprimirTarjeta: vi.fn(),
}));

function pinta() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<Provider><QueryClientProvider client={qc}><MemoryRouter><VentaEnWebSection /></MemoryRouter><Toaster /></QueryClientProvider></Provider>);
}

beforeEach(() => {
  update.mockReset();
  qrSvg.mockClear();
});

describe('Ajustes › Venta en web (maqueta B)', () => {
  it('muestra la dirección guardada, el QR del código estable y las descargas', async () => {
    pinta();
    expect(await screen.findByText('menu.test/fonda-dona-lupe')).toBeInTheDocument();
    await waitFor(() => expect(qrSvg).toHaveBeenCalledWith('https://menu.test/q/0123456789ab'));
    for (const n of [/copiar enlace/i, /compartir/i, /png/i, /svg/i, /imprimir tarjeta/i]) {
      expect(screen.getByRole('button', { name: n })).toBeInTheDocument();
    }
    expect(screen.getByRole('link', { name: /abrir/i })).toHaveAttribute('href', 'https://menu.test/fonda-dona-lupe');
  });

  it('copiar cambia a «Copiado»', async () => {
    const writeText = vi.fn(() => Promise.resolve());
    Object.assign(navigator, { clipboard: { writeText } });
    pinta();
    fireEvent.click(await screen.findByRole('button', { name: /copiar enlace/i }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('https://menu.test/fonda-dona-lupe'));
    expect(await screen.findByRole('button', { name: /copiado/i })).toBeInTheDocument();
  });

  it('el nombre se ve como quedará mientras se escribe, y lo corto avisa en vivo', async () => {
    pinta();
    const campo = await screen.findByLabelText(/nombre en la dirección/i);
    fireEvent.change(campo, { target: { value: 'Doña Lupe' } });
    expect(screen.getByText('dona-lupe')).toBeInTheDocument();
    expect(screen.getByText('menu.test/fonda-dona-lupe')).toBeInTheDocument();
    fireEvent.change(campo, { target: { value: 'ñ' } });
    expect(screen.getByText(/usa al menos 3 letras o números/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^guardar$/i })).toBeDisabled();
  });

  it('un nombre que no está disponible lo dice bajo el campo', async () => {
    update.mockRejectedValue(new ApiError(409, 'MENU_NAME_UNAVAILABLE', 'ese nombre no está disponible', 'r'));
    pinta();
    fireEvent.change(await screen.findByLabelText(/nombre en la dirección/i), { target: { value: 'admin' } });
    fireEvent.click(screen.getByRole('button', { name: /^guardar$/i }));
    expect(await screen.findByText(/ese nombre no está disponible/i)).toBeInTheDocument();
    expect(update).toHaveBeenCalledWith({ name: 'admin' });
  });

  it('el color es un grupo con nombre, dice cuál está elegido y se guarda al tocarlo', async () => {
    update.mockResolvedValue({ ...ajustes, palette: 'green' });
    pinta();
    const grupo = await screen.findByRole('radiogroup', { name: /color del menú/i });
    expect(grupo).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Gato Bobah' })).toHaveAttribute('aria-checked', 'true');
    fireEvent.click(screen.getByRole('radio', { name: 'Bosque' }));
    await waitFor(() => expect(update).toHaveBeenCalledWith({ palette: 'green' }));
    expect(await screen.findByText(/color guardado/i)).toBeInTheDocument();
  });

  it('si el portapapeles no deja copiar, lo dice', async () => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn(() => Promise.reject(new Error('no'))) } });
    pinta();
    fireEvent.click(await screen.findByRole('button', { name: /copiar enlace/i }));
    expect(await screen.findByText(/no se pudo copiar/i)).toBeInTheDocument();
  });

  it('si el QR no se puede generar, lo dice y deja reintentar', async () => {
    qrSvg.mockRejectedValueOnce(new Error('chunk'));
    pinta();
    expect(await screen.findByText(/no se pudo generar el código qr/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /reintentar/i }));
    await waitFor(() => expect(qrSvg).toHaveBeenCalledTimes(2));
  });

  it('al guardar el nombre lo confirma', async () => {
    update.mockResolvedValue({ ...ajustes, name: 'dona-lupe' });
    pinta();
    fireEvent.change(await screen.findByLabelText(/nombre en la dirección/i), { target: { value: 'Doña Lupe' } });
    fireEvent.click(screen.getByRole('button', { name: /^guardar$/i }));
    expect(await screen.findByText(/nombre guardado/i)).toBeInTheDocument();
  });

  it('los colores son una sola parada de teclado y se recorren con flechas', async () => {
    update.mockResolvedValue({ ...ajustes, palette: 'blue' });
    pinta();
    await screen.findByRole('radiogroup', { name: /color del menú/i });
    const radios = screen.getAllByRole('radio');
    expect(radios.filter((r) => r.getAttribute('tabindex') === '0')).toHaveLength(1);
    fireEvent.keyDown(screen.getByRole('radio', { name: 'Gato Bobah' }), { key: 'ArrowRight' });
    await waitFor(() => expect(update).toHaveBeenCalledWith({ palette: 'blue' }));
  });

  it('el resumen de productos queda en un renglón con su acceso', async () => {
    pinta();
    expect(await screen.findByText(/142/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /ir a los productos/i })).toBeInTheDocument();
  });
});
