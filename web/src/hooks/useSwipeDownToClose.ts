import { useRef, useState } from 'react';
import type { PointerEvent as ReactPointerEvent } from 'react';

const DISMISS_THRESHOLD = 80; // px arrastrados hacia abajo para cerrar

// Si el drag arranca sobre un control interactivo (buscar opción, botón "ver archivadas"…)
// lo dejamos pasar sin capturar el pointer, para no robarle el tap/foco a esos controles.
const INTERACTIVE_SELECTOR = 'input, textarea, button, select, [role="button"], a';

// Handlers de pointer events + el offset en vivo para arrastrar hacia abajo y cerrar un
// modal/drawer (bottom sheet táctil). Pensados para un área amplia (p. ej. el header
// completo), no solo un grip angosto — así el touch target es lo bastante grande para un
// dedo en tablet. Solo cierra hacia abajo: arrastrar hacia arriba se clampa a 0. Pointer
// capture: el arrastre sigue funcionando aunque el dedo se salga del área mientras se mueve.
export function useSwipeDownToClose(onClose: () => void) {
  const [offset, setOffset] = useState(0);
  const [dragging, setDragging] = useState(false); // solo para la transición CSS (se lee en render)
  // Guard SÍNCRONO del arrastre en un ref: los tres eventos (down/move/up) pueden llegar en el
  // mismo tick sin re-render entre ellos; con state, move/up leerían un `dragging` viejo (false)
  // y el gesto nunca cerraría. El ref se ve al instante en los handlers.
  const activeRef = useRef(false);
  const offsetRef = useRef(0);
  const startY = useRef(0);

  const onPointerDown = (e: ReactPointerEvent<HTMLElement>) => {
    if (e.target instanceof HTMLElement && e.target.closest(INTERACTIVE_SELECTOR)) return;
    activeRef.current = true;
    setDragging(true);
    startY.current = e.clientY;
    e.currentTarget.setPointerCapture(e.pointerId);
  };
  const onPointerMove = (e: ReactPointerEvent<HTMLElement>) => {
    if (!activeRef.current) return;
    const next = Math.max(0, e.clientY - startY.current);
    offsetRef.current = next;
    setOffset(next);
  };
  const endDrag = () => {
    if (!activeRef.current) return;
    activeRef.current = false;
    setDragging(false);
    const dismiss = offsetRef.current > DISMISS_THRESHOLD;
    offsetRef.current = 0;
    setOffset(0);
    if (dismiss) onClose();
  };

  return {
    offset,
    dragging,
    handlers: {
      onPointerDown,
      onPointerMove,
      onPointerUp: endDrag,
      onPointerCancel: endDrag,
    },
  };
}

// Tipo de los handlers, para componentes que reciben el gesto ya armado desde el padre
// (p. ej. Ticket, reusado dentro y fuera de un drawer) sin acoplarse al hook mismo.
export type SwipeHandlers = ReturnType<typeof useSwipeDownToClose>['handlers'];
