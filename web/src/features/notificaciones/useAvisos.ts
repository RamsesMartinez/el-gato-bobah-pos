import { useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { notificationsApi } from '../../api/notifications';
import { useSessionStore } from '../../stores/session';
import { alReconectar } from '../../hooks/alReconectar';
import { can } from '../../app/permissions';

const BASE = import.meta.env.VITE_API_URL || '/api/v1';

// Una sola conexión en vivo por pantalla, la compartan cuantos componentes la usen (la campana y la
// franja): con una por componente cada aviso refrescaba dos veces y la tableta sostenía dos conexiones.
let enVivo: { token: string; es: EventSource; usuarios: number } | null = null;

function suscribir(token: string, refresca: () => void): () => void {
  if (!enVivo || enVivo.token !== token) {
    enVivo?.es.close();
    const es = new EventSource(`${BASE}/events?token=${encodeURIComponent(token)}`);
    es.addEventListener('notifications.changed', refresca);
    alReconectar(es, refresca);
    enVivo = { token, es, usuarios: 0 };
  }
  const propia = enVivo;
  propia.usuarios++;
  return () => {
    propia.usuarios--;
    if (propia.usuarios === 0) {
      propia.es.close();
      if (enVivo === propia) enVivo = null;
    }
  };
}

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
    return suscribir(token, () => void qc.invalidateQueries({ queryKey: ['avisos'] }));
  }, [enabled, token, qc]);

  const unread = useQuery({ queryKey: ['avisos', 'sin-leer'], queryFn: notificationsApi.unread, enabled, refetchInterval: 60_000 });
  const open = useQuery({ queryKey: ['avisos', 'open'], queryFn: () => notificationsApi.list('open'), enabled, refetchInterval: 60_000 });
  return { enabled, unread: unread.data?.unread ?? 0, open: open.data?.items ?? [] };
}
