import { useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useSessionStore } from '../stores/session';

const BASE = import.meta.env.VITE_API_URL || '/api/v1';

// Suscribe el tablero a los eventos SSE del backend; invalida la query de órdenes
// activas ante cada cambio. Devuelve el estado de conexión en vivo.
export function useOrderEvents(): boolean {
  const qc = useQueryClient();
  const token = useSessionStore((s) => s.token);
  const [live, setLive] = useState(false);

  useEffect(() => {
    if (!token) return;
    const es = new EventSource(`${BASE}/events?token=${encodeURIComponent(token)}`);
    const invalidate = () => qc.invalidateQueries({ queryKey: ['orders', 'active'] });

    es.addEventListener('order.created', invalidate);
    es.addEventListener('order.updated', invalidate);
    es.onopen = () => setLive(true);
    es.onerror = () => setLive(false);

    return () => es.close();
  }, [token, qc]);

  return live;
}
