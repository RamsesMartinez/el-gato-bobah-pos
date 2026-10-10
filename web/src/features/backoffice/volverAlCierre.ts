import { useRef } from 'react';

// useVolverAlCierre: «Entregar ahora» desde el cierre marca el viaje; al entregar, `volver` regresa
// la pantalla al cierre. Una entrega iniciada en otro lado no mueve la pantalla.
export function useVolverAlCierre<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const pendiente = useRef(false);
  return {
    ref,
    marcar: () => { pendiente.current = true; },
    volver: () => {
      if (!pendiente.current) return;
      pendiente.current = false;
      ref.current?.scrollIntoView({ block: 'center', behavior: 'smooth' });
    },
  };
}
