import { useCallback, useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { pedidosEntrantes, type PedidoEntrante } from '../../api/pedidosEntrantes';
import { useSessionStore } from '../../stores/session';
import { can } from '../../app/permissions';

const BASE = import.meta.env.VITE_API_URL || '/api/v1';

/** La llave de caché, exportada para invalidarla desde quien acepta o rechaza. */
export const CLAVE_ENTRANTES = ['pedidos-entrantes'] as const;

/** El sondeo de abajo. El criterio de 5 s lo cumplen el aviso, la reconexión y el primer plano. */
export const INTERVALO_DE_SONDEO = 15_000;

/**
 * useEntrantes trae los pedidos que esperan decisión y se entera de cada cambio. La regla de qué
 * está pendiente es del servidor; esto solo pregunta a tiempo.
 *
 * CUATRO CAMINOS PARA ENTERARSE, y ninguno sobra:
 *   - el aviso `incoming.changed` por /events: el caso normal, en menos de un segundo;
 *   - volver a pedir al reconectar el canal: red caída o una subida que cambió de copia de la API;
 *   - volver a pedir al regresar a primer plano: la tableta que se suspendió;
 *   - el sondeo cada 15 s: un canal que sigue «abierto» pero ya no entrega nada.
 *
 * Un error de red NO borra lo que ya se sabía (TanStack conserva los datos): la alarma sigue
 * sonando sin red, que es lo que pide FR-021.
 */
export function useEntrantes({ escuchar = false }: { escuchar?: boolean } = {}): { pedidos: PedidoEntrante[]; refrescar: () => void } {
  const token = useSessionStore((s) => s.token);
  const user = useSessionStore((s) => s.user);
  const habilitado = !!token && can('orders.accept_incoming', user);
  const qc = useQueryClient();

  const refrescar = useCallback(() => {
    void qc.invalidateQueries({ queryKey: CLAVE_ENTRANTES });
  }, [qc]);

  useEffect(() => {
    // UN SOLO CANAL POR PESTAÑA: lo abre la alerta del shell, que siempre está montada; la columna y
    // el tablero leen la misma consulta sin abrir el suyo.
    if (!escuchar || !habilitado || !token) return () => {};
    const es = new EventSource(`${BASE}/events?token=${encodeURIComponent(token)}`);
    es.addEventListener('incoming.changed', refrescar);
    // En CADA apertura y no solo al reconectar: al renovar el token este canal se cierra y se abre
    // otro, y lo que llegó en ese hueco no vuelve a avisarse.
    es.addEventListener('open', refrescar);
    const alVolver = () => {
      if (document.visibilityState === 'visible') refrescar();
    };
    document.addEventListener('visibilitychange', alVolver);
    return () => {
      es.close();
      document.removeEventListener('visibilitychange', alVolver);
    };
  }, [escuchar, habilitado, token, refrescar]);

  const { data } = useQuery({
    queryKey: CLAVE_ENTRANTES,
    queryFn: pedidosEntrantes,
    enabled: habilitado,
    refetchInterval: INTERVALO_DE_SONDEO,
    // Una tableta con la pantalla encendida pero en otra pestaña también tiene que enterarse.
    refetchIntervalInBackground: true,
    retry: false,
  });

  return { pedidos: habilitado ? (data ?? []) : [], refrescar };
}
