import { useLayoutEffect, useRef, useState } from 'react';

// Mide el ancho del contenedor (no del viewport) para decidir el layout del POS.
export function useContainerWidth<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  // Init con innerWidth (aprox.) para acertar el layout ya en el primer render, y medir el ancho
  // real en useLayoutEffect (ANTES del paint): sin ese timing, width=0 pinta la barra inferior un
  // frame y "parpadea" a la píldora en cada refresh.
  const [width, setWidth] = useState(() => (typeof window !== 'undefined' ? window.innerWidth : 0));

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      setWidth(entries[0].contentRect.width);
    });
    ro.observe(el);
    setWidth(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  return { ref, width };
}
