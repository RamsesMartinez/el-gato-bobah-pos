import { act, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, test } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { AvisoSinConexion } from './AvisoSinConexion';
import { reportarResultado } from './useSinConexion';

function red(enLinea: boolean) {
  Object.defineProperty(window.navigator, 'onLine', { configurable: true, get: () => enLinea });
  window.dispatchEvent(new Event(enLinea ? 'online' : 'offline'));
}

beforeEach(() => { red(true); reportarResultado(null); });

describe('el aviso de sin conexión (US7, lienzo V2-5)', () => {
  test('con conexión no se pinta nada', () => {
    render(<Provider><AvisoSinConexion /></Provider>);
    expect(screen.queryByRole('alert')).toBeNull();
  });

  test('sin conexión dice qué no se puede hacer y que se reintenta solo', () => {
    render(<Provider><AvisoSinConexion /></Provider>);
    act(() => red(false));
    const aviso = screen.getByRole('alert');
    expect(aviso).toHaveTextContent('Sin conexión');
    expect(aviso).toHaveTextContent(/no se puede agregar, mandar a cocina ni cobrar/i);
    expect(aviso).toHaveTextContent(/se reintenta solo/i);
  });

  // Va ENCIMA de la franja del encabezado: si empujara la pantalla, a 600 px de alto se llevaría
  // un renglón de productos y movería los botones bajo el dedo justo cuando la red parpadea.
  test('va superpuesto: no ocupa alto en la página', () => {
    render(<Provider><AvisoSinConexion /></Provider>);
    act(() => red(false));
    const estilo = getComputedStyle(screen.getByRole('alert'));
    expect(['absolute', 'fixed']).toContain(estilo.position);
  });

  test('desaparece solo al volver la conexión', () => {
    render(<Provider><AvisoSinConexion /></Provider>);
    act(() => reportarResultado(new TypeError('Failed to fetch')));
    expect(screen.getByRole('alert')).toBeInTheDocument();
    act(() => reportarResultado(null));
    expect(screen.queryByRole('alert')).toBeNull();
  });
});

// La revisión de tableta: el aviso va encima del encabezado del ticket; no puede robarse los
// toques de «Ocultar pedido» ni del ⋮ que quedan debajo.
test('no captura toques', () => {
  render(<Provider><AvisoSinConexion /></Provider>);
  act(() => reportarResultado(new TypeError('Failed to fetch')));
  expect(getComputedStyle(screen.getByRole('alert')).pointerEvents).toBe('none');
});
