import { useCallback, useSyncExternalStore } from 'react';

// Anchos de pantalla grande para Caja (#290, fase 4). Van aparte de los breakpoints de Chakra, que el
// resto del POS mide y no se mueven: PC desde 1440 px (tres columnas) y 2K desde 2560 px (cajas lado a
// lado).
export const ANCHO_PC = 1440;
export const ANCHO_2K = 2560;
export const desde = (px: number) => `@media screen and (min-width: ${px}px)`;

// useAnchoMinimo dice si la ventana mide al menos `px` y se actualiza al cambiar de tamaño.
export function useAnchoMinimo(px: number): boolean {
  const query = `(min-width: ${px}px)`;
  // Estable por consulta: una función nueva en cada render quitaba y volvía a poner el listener.
  const suscribir = useCallback((cambio: () => void) => {
    if (typeof window === 'undefined' || !window.matchMedia) return () => {};
    const m = window.matchMedia(query);
    m.addEventListener('change', cambio);
    return () => m.removeEventListener('change', cambio);
  }, [query]);
  return useSyncExternalStore(
    suscribir,
    () => (typeof window !== 'undefined' && !!window.matchMedia && window.matchMedia(query).matches),
    () => false,
  );
}
