import { useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { notificationsApi } from '../../api/notifications';
import { useSessionStore } from '../../stores/session';
import { alReconectar } from '../../hooks/alReconectar';
import { can } from '../../app/permissions';

const BASE = import.meta.env.VITE_API_URL || '/api/v1';

// useAvisos mantiene al día la campana: el número sin leer y los abiertos. Lo nuevo llega en vivo
// (`notifications.changed`, que la API publica cuando la base abre o cierra un aviso); si la conexión
// se cae, se vuelve a pedir al reconectar y, por si acaso, cada minuto.
export function useAvisos() {
  const token = useSessionStore((s) => s.token);
  const user = useSessionStore((s) => s.user);
  const qc = useQueryClient();
  const enabled = !!token && can('inventory.report_out', user);

  useEffect(() => {
    if (!enabled || !token) return;
    const refresca = () => void qc.invalidateQueries({ queryKey: ['avisos'] });
    const es = new EventSource(`${BASE}/events?token=${encodeURIComponent(token)}`);
    es.addEventListener('notifications.changed', refresca);
    alReconectar(es, refresca);
    return () => es.close();
  }, [enabled, token, qc]);

  const unread = useQuery({ queryKey: ['avisos', 'sin-leer'], queryFn: notificationsApi.unread, enabled, refetchInterval: 60_000 });
  const open = useQuery({ queryKey: ['avisos', 'open'], queryFn: () => notificationsApi.list('open'), enabled, refetchInterval: 60_000 });
  return { enabled, unread: unread.data?.unread ?? 0, open: open.data?.items ?? [] };
}
