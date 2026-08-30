import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import type { PointerEvent as ReactPointerEvent } from 'react';
import { useLongPress } from './useLongPress';

function pointerEvent(clientX = 0, clientY = 0) {
  return { clientX, clientY, pointerId: 1 } as unknown as ReactPointerEvent<HTMLElement>;
}

describe('useLongPress', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('dispara la acción larga al mantener presionado, y no la corta', () => {
    const largo = vi.fn();
    const corto = vi.fn();
    const { result } = renderHook(() => useLongPress(largo, corto));
    act(() => {
      result.current.onPointerDown(pointerEvent());
      vi.advanceTimersByTime(600);
      result.current.onPointerUp();
      result.current.onClick();
    });
    expect(largo).toHaveBeenCalledTimes(1);
    expect(corto).not.toHaveBeenCalled();
  });

  it('un toque normal dispara la acción corta', () => {
    const largo = vi.fn();
    const corto = vi.fn();
    const { result } = renderHook(() => useLongPress(largo, corto));
    act(() => {
      result.current.onPointerDown(pointerEvent());
      vi.advanceTimersByTime(120);
      result.current.onPointerUp();
      result.current.onClick();
    });
    expect(corto).toHaveBeenCalledTimes(1);
    expect(largo).not.toHaveBeenCalled();
  });

  // En una tablet el dedo nunca se queda quieto. Un umbral de movimiento evita que un scroll del
  // catálogo abra el diálogo de precio del producto por el que pasó el dedo.
  it('un arrastre cancela las dos acciones', () => {
    const largo = vi.fn();
    const corto = vi.fn();
    const { result } = renderHook(() => useLongPress(largo, corto));
    act(() => {
      result.current.onPointerDown(pointerEvent(0, 0));
      result.current.onPointerMove(pointerEvent(0, 40));
      vi.advanceTimersByTime(600);
      result.current.onPointerUp();
      result.current.onClick();
    });
    expect(largo).not.toHaveBeenCalled();
    expect(corto).not.toHaveBeenCalled();
  });

  it('un temblor pequeño no cancela nada', () => {
    const largo = vi.fn();
    const corto = vi.fn();
    const { result } = renderHook(() => useLongPress(largo, corto));
    act(() => {
      result.current.onPointerDown(pointerEvent(0, 0));
      result.current.onPointerMove(pointerEvent(3, 4)); // 5px
      vi.advanceTimersByTime(600);
      result.current.onPointerUp();
      result.current.onClick();
    });
    expect(largo).toHaveBeenCalledTimes(1);
    expect(corto).not.toHaveBeenCalled();
  });

  // Salir del control con el dedo abajo es un arrepentimiento: ni corta ni larga.
  it('cancelar deja todo sin disparar', () => {
    const largo = vi.fn();
    const corto = vi.fn();
    const { result } = renderHook(() => useLongPress(largo, corto));
    act(() => {
      result.current.onPointerDown(pointerEvent());
      result.current.onPointerCancel();
      vi.advanceTimersByTime(600);
    });
    expect(largo).not.toHaveBeenCalled();
    expect(corto).not.toHaveBeenCalled();
  });

  // Sin acción larga (mostrador, o sin permiso) el control es un botón normal.
  it('sin acción larga el toque sigue funcionando', () => {
    const corto = vi.fn();
    const { result } = renderHook(() => useLongPress(undefined, corto));
    act(() => {
      result.current.onPointerDown(pointerEvent());
      vi.advanceTimersByTime(600);
      result.current.onPointerUp();
      result.current.onClick();
    });
    expect(corto).toHaveBeenCalledTimes(1);
  });

  // Enter y Espacio sobre un <button> disparan click sin un solo evento de puntero. Si la acción
  // corta colgara del onPointerUp, el mosaico sería inservible con teclado.
  it('el click solo, sin puntero, cuenta como toque', () => {
    const largo = vi.fn();
    const corto = vi.fn();
    const { result } = renderHook(() => useLongPress(largo, corto));
    act(() => { result.current.onClick(); });
    expect(corto).toHaveBeenCalledTimes(1);
  });

  // En tablet, tras una pulsación larga el click puede no llegar nunca. Si el flag no se limpiara
  // en el siguiente onPointerDown, el toque de después se comería en silencio.
  it('tras una pulsación larga sin click, el toque siguiente sí cuenta', () => {
    const largo = vi.fn();
    const corto = vi.fn();
    const { result } = renderHook(() => useLongPress(largo, corto));
    act(() => {
      result.current.onPointerDown(pointerEvent());
      vi.advanceTimersByTime(600);
      result.current.onPointerUp(); // sin click: el navegador no lo emitió
    });
    act(() => {
      result.current.onPointerDown(pointerEvent());
      vi.advanceTimersByTime(120);
      result.current.onPointerUp();
      result.current.onClick();
    });
    expect(corto).toHaveBeenCalledTimes(1);
  });
});
