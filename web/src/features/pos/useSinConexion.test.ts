import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test } from 'vitest';
import { ApiError } from '../../api/client';
import { esFalloDeRed, reportarResultado, useSinConexion } from './useSinConexion';

function navegador(enLinea: boolean) {
  Object.defineProperty(window.navigator, 'onLine', { configurable: true, get: () => enLinea });
  window.dispatchEvent(new Event(enLinea ? 'online' : 'offline'));
}

beforeEach(() => { navegador(true); reportarResultado(null); });
afterEach(() => navegador(true));

describe('sin conexión (D-4: capturar exige red)', () => {
  test('el navegador dice offline → sin conexión', () => {
    const { result } = renderHook(() => useSinConexion());
    expect(result.current).toBe(false);
    act(() => navegador(false));
    expect(result.current).toBe(true);
  });

  // El wifi del local se cae «hacia adentro»: el navegador sigue diciendo onLine (hay red local) y
  // ninguna petición llega. Fiarse de navigator.onLine dejaría Cobrar encendido sin servidor.
  test('un fallo de red en una mutación → sin conexión aunque navigator.onLine diga que sí', () => {
    const { result } = renderHook(() => useSinConexion());
    act(() => reportarResultado(new TypeError('Failed to fetch')));
    expect(result.current).toBe(true);
  });

  test('vuelve sola al primer éxito', () => {
    const { result } = renderHook(() => useSinConexion());
    act(() => reportarResultado(new TypeError('Failed to fetch')));
    act(() => reportarResultado(null));
    expect(result.current).toBe(false);
  });

  // Un rechazo del servidor PRUEBA que hay conexión: no puede encender el aviso.
  test('un rechazo del servidor no es falta de conexión, y la devuelve', () => {
    const { result } = renderHook(() => useSinConexion());
    act(() => reportarResultado(new TypeError('Failed to fetch')));
    act(() => reportarResultado(new ApiError(409, 'DRAFT_CHANGED', 'x', 'r')));
    expect(result.current).toBe(false);
  });

  test('esFalloDeRed distingue la red del servidor', () => {
    expect(esFalloDeRed(new TypeError('Failed to fetch'))).toBe(true);
    expect(esFalloDeRed(new ApiError(500, 'INTERNAL', 'x', 'r'))).toBe(false);
    // Así llega una caída de red desde api/client: estado 0, código NETWORK.
    expect(esFalloDeRed(new ApiError(0, 'NETWORK', 'Sin conexión con el servidor', 'r'))).toBe(true);
  });
});
