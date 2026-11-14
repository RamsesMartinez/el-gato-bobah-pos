import { renderHook } from '@testing-library/react';
import { vi } from 'vitest';
import { useVolverAlCierre } from './volverAlCierre';

// «Entregar ahora» desde el cierre abre la hoja de reparto; al entregar, la pantalla tiene que
// regresar al cierre en vez de dejar a quien cierra donde estaba la hoja, buscando el botón.
test('tras entregar desde el cierre, regresa al cierre una sola vez', () => {
  const { result } = renderHook(() => useVolverAlCierre<HTMLDivElement>());
  const el = document.createElement('div');
  el.scrollIntoView = vi.fn();
  (result.current.ref as { current: HTMLDivElement | null }).current = el;
  result.current.marcar();
  result.current.volver();
  expect(el.scrollIntoView).toHaveBeenCalledTimes(1);
  result.current.volver();
  expect(el.scrollIntoView).toHaveBeenCalledTimes(1);
});

test('una entrega que no empezó en el cierre no mueve la pantalla', () => {
  const { result } = renderHook(() => useVolverAlCierre<HTMLDivElement>());
  const el = document.createElement('div');
  el.scrollIntoView = vi.fn();
  (result.current.ref as { current: HTMLDivElement | null }).current = el;
  result.current.volver();
  expect(el.scrollIntoView).not.toHaveBeenCalled();
});
