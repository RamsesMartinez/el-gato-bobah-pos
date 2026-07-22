import { describe, it, expect, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import type { PointerEvent as ReactPointerEvent } from 'react';
import { useSwipeDownToClose } from './useSwipeDownToClose';

function pointerEvent(clientY: number, target: Element = document.createElement('div')) {
  return {
    clientY,
    pointerId: 1,
    target,
    currentTarget: { setPointerCapture: vi.fn() },
  } as unknown as ReactPointerEvent<HTMLElement>;
}

describe('useSwipeDownToClose', () => {
  it('cierra al arrastrar más allá del umbral', () => {
    const onClose = vi.fn();
    const { result } = renderHook(() => useSwipeDownToClose(onClose));
    act(() => {
      result.current.handlers.onPointerDown(pointerEvent(100));
      result.current.handlers.onPointerMove(pointerEvent(250)); // 150px hacia abajo
      result.current.handlers.onPointerUp();
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(result.current.offset).toBe(0);
  });

  it('regresa a su lugar sin cerrar en un arrastre corto', () => {
    const onClose = vi.fn();
    const { result } = renderHook(() => useSwipeDownToClose(onClose));
    act(() => {
      result.current.handlers.onPointerDown(pointerEvent(100));
      result.current.handlers.onPointerMove(pointerEvent(130)); // 30px, bajo el umbral
      result.current.handlers.onPointerUp();
    });
    expect(onClose).not.toHaveBeenCalled();
    expect(result.current.offset).toBe(0);
  });

  it('clampa arrastres hacia arriba a 0 (solo cierra hacia abajo)', () => {
    const onClose = vi.fn();
    const { result } = renderHook(() => useSwipeDownToClose(onClose));
    act(() => {
      result.current.handlers.onPointerDown(pointerEvent(100));
      result.current.handlers.onPointerMove(pointerEvent(20)); // hacia arriba
    });
    expect(result.current.offset).toBe(0);
  });

  it('expone dragging=true mientras se arrastra y false tras soltar', () => {
    const onClose = vi.fn();
    const { result } = renderHook(() => useSwipeDownToClose(onClose));
    expect(result.current.dragging).toBe(false);
    act(() => result.current.handlers.onPointerDown(pointerEvent(100)));
    expect(result.current.dragging).toBe(true);
    act(() => result.current.handlers.onPointerMove(pointerEvent(150)));
    expect(result.current.dragging).toBe(true);
    act(() => result.current.handlers.onPointerUp());
    expect(result.current.dragging).toBe(false);
  });

  it('ignora pointermove antes de un pointerdown', () => {
    const onClose = vi.fn();
    const { result } = renderHook(() => useSwipeDownToClose(onClose));
    act(() => {
      result.current.handlers.onPointerMove(pointerEvent(300));
    });
    expect(result.current.offset).toBe(0);
  });

  it('no arrastra si el pointerdown arranca sobre un control interactivo', () => {
    const onClose = vi.fn();
    const { result } = renderHook(() => useSwipeDownToClose(onClose));
    const input = document.createElement('input');
    act(() => {
      result.current.handlers.onPointerDown(pointerEvent(100, input));
      result.current.handlers.onPointerMove(pointerEvent(250, input)); // 150px, superaría el umbral
      result.current.handlers.onPointerUp();
    });
    expect(result.current.dragging).toBe(false);
    expect(onClose).not.toHaveBeenCalled();
    expect(result.current.offset).toBe(0);
  });
});
