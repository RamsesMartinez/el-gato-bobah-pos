import { useCallback, useEffect, useRef } from 'react';
import type { PointerEvent as ReactPointerEvent } from 'react';

// Mantener presionado = acción secundaria. Es el gesto que no cuesta un tap en el flujo normal:
// la vara de UX del POS es minimizar taps, y agregar un botón de "editar precio" a cada mosaico
// robaría espacio en una pantalla de 7" para algo que se usa una vez por producto.
const MS = 450;
// 12 px de tolerancia: en una tablet el dedo nunca se queda quieto, pero un scroll del catálogo
// recorre mucho más. Sin este umbral, deslizar la lista abriría el diálogo del producto por el que
// pasó el dedo.
const MOVIMIENTO_MAX = 12;

// useLongPress devuelve los handlers para un control con dos acciones. `largo` puede ser undefined
// (mostrador, o sin permiso): entonces el control se comporta como un botón normal.
//
// La acción corta sale de onClick y NO de onPointerUp: Enter y Espacio sobre un <button> disparan
// click sin ningún evento de puntero, así que colgarla del puntero dejaría el mosaico inservible
// con teclado. El gesto largo solo SUPRIME ese click.
export function useLongPress(largo: (() => void) | undefined, corto: () => void) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const disparado = useRef(false);
  const origen = useRef({ x: 0, y: 0 });

  const cancelar = useCallback(() => {
    if (timer.current) {
      clearTimeout(timer.current);
      timer.current = null;
    }
  }, []);

  // Un desmontaje con el dedo abajo —cambiar de categoría, que el menú se refresque— dejaría el
  // timer vivo llamando a un callback de un componente que ya no existe.
  useEffect(() => cancelar, [cancelar]);

  const onPointerDown = useCallback((e: ReactPointerEvent<HTMLElement>) => {
    disparado.current = false;
    origen.current = { x: e.clientX, y: e.clientY };
    if (!largo) return;
    timer.current = setTimeout(() => {
      disparado.current = true;
      timer.current = null;
      largo();
    }, MS);
  }, [largo]);

  const onPointerMove = useCallback((e: ReactPointerEvent<HTMLElement>) => {
    const dx = e.clientX - origen.current.x;
    const dy = e.clientY - origen.current.y;
    if (Math.hypot(dx, dy) > MOVIMIENTO_MAX) {
      cancelar();
      // Se marca como disparado para que el onPointerUp del final del arrastre tampoco cuente como
      // toque: quien desliza el catálogo no quiere agregar el producto que quedó bajo el dedo.
      disparado.current = true;
    }
  }, [cancelar]);

  const onPointerUp = useCallback(() => {
    // Solo se apaga el timer. El flag NO se limpia aquí: el click llega después y necesita saber
    // si el gesto largo ya corrió. Lo limpia el siguiente onPointerDown, que también cubre el caso
    // de las tablets donde tras una pulsación larga el click nunca llega.
    cancelar();
  }, [cancelar]);

  const onPointerCancel = useCallback(() => {
    cancelar();
    disparado.current = true;
  }, [cancelar]);

  const onClick = useCallback(() => {
    if (disparado.current) {
      disparado.current = false;
      return;
    }
    corto();
  }, [corto]);

  return { onPointerDown, onPointerMove, onPointerUp, onPointerCancel, onClick };
}
