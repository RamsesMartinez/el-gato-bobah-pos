import { useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useSessionStore } from '../stores/session';

const BASE = import.meta.env.VITE_API_URL || '/api/v1';

// Suscribe el tablero a los eventos SSE del backend; refresca las listas de pedidos ante cada
// cambio. Devuelve el estado de conexión en vivo.
export function useOrderEvents(): boolean {
  const qc = useQueryClient();
  const token = useSessionStore((s) => s.token);
  const [live, setLive] = useState(false);

  useEffect(() => {
    if (!token) return;
    const es = new EventSource(`${BASE}/events?token=${encodeURIComponent(token)}`);
    // TODAS las listas de pedidos, no solo la del tablero de cocina.
    //
    // Invalidaba `['orders','active']` a secas, así que cobrar desde el tablero dejaba la barra del
    // POS —que lee `['orders','open']`— contando ese dinero hasta 30 segundos, que es su intervalo
    // de refresco. El operador veía dos pendientes distintos del mismo pedido en dos pantallas y no
    // tenía cómo saber cuál era el bueno; con cobro parcial la diferencia deja de ser un retraso y
    // pasa a ser una cifra equivocada.
    const invalidate = () => qc.invalidateQueries({ queryKey: ['orders'] });

    es.addEventListener('order.created', invalidate);
    es.addEventListener('order.updated', invalidate);
    es.onopen = () => setLive(true);
    es.onerror = () => setLive(false);

    return () => es.close();
  }, [token, qc]);

  return live;
}
