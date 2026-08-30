import { useEffect } from 'react';

import { useSessionStore } from '../stores/session';
import { useReprecio } from './useReprecio';

const BASE = import.meta.env.VITE_API_URL || '/api/v1';

// Escucha los cambios de catálogo que hizo OTRA tablet.
//
// El backend ya publicaba `menu.updated` en cada corrección de precio, pero nadie lo escuchaba: el
// menú se quedaba cacheado hasta cinco minutos, así que la segunda tablet podía armar un ticket con
// el precio viejo mientras el servidor cobraba el nuevo. Total impreso distinto del cobrado, que es
// exactamente lo que la invalidación del servidor buscaba evitar.
//
// Va en la pantalla de venta y no en el tablero porque es ahí donde el precio equivocado cuesta
// dinero.
export function useMenuEvents(): void {
  const token = useSessionStore((s) => s.token);
  const reprecia = useReprecio();

  useEffect(() => {
    if (!token) return;
    const es = new EventSource(`${BASE}/events?token=${encodeURIComponent(token)}`);
    es.addEventListener('menu.updated', () => {
      void reprecia();
    });
    return () => es.close();
  }, [token, reprecia]);
}
