import { useCallback, useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { pedidosPendientes } from '../../api/pedidosDePlataforma';
import { useSessionStore } from '../../stores/session';

const BASE = import.meta.env.VITE_API_URL || '/api/v1';

/** La llave de caché, exportada para poder invalidarla desde quien acepta. */
export const CLAVE_PENDIENTES = ['pedidos-de-plataforma', 'pendientes'] as const;

/**
 * usePedidosDePlataforma trae los pedidos que esperan decisión y se entera en vivo de los nuevos.
 *
 * DOS CAMINOS A PROPÓSITO, y no es redundancia:
 *
 *   - El evento en vivo es el que hace que el pedido aparezca en segundos, que es el criterio de
 *     esta feature (SC-001).
 *   - El refresco periódico es la red de abajo. Si la conexión de eventos se cae —una tableta que
 *     se durmió, un wifi que parpadeó— el evento se pierde y NO hay forma de recuperarlo: el pedido
 *     se queda invisible hasta que la plataforma lo cancela sola y el cliente reclama. Un fallo
 *     silencioso que solo se descubre por el cliente es justo el que hay que cerrar dos veces.
 *
 * El intervalo es de 30 segundos: el plazo total es de 11.5 minutos, así que en el peor caso —el
 * evento perdido— todavía quedan diez minutos para decidir.
 */
export function usePedidosDePlataforma() {
  const token = useSessionStore((s) => s.token);
  const qc = useQueryClient();

  const refrescar = useCallback(() => {
    void qc.invalidateQueries({ queryKey: CLAVE_PENDIENTES });
  }, [qc]);

  useEffect(() => {
    if (!token) return () => {};
    const es = new EventSource(`${BASE}/events?token=${encodeURIComponent(token)}`);
    es.addEventListener('platform.order.received', refrescar);
    // Cuando OTRA tableta ya lo atendió. Sin esto, la segunda sigue mostrando la alarma y quien la
    // toque encuentra un botón que ya no hace nada.
    es.addEventListener('platform.order.decided', refrescar);
    return () => es.close();
  }, [token, refrescar]);

  return useQuery({
    queryKey: CLAVE_PENDIENTES,
    queryFn: pedidosPendientes,
    refetchInterval: 30000,
    // Sin pedidos pendientes la lista es vacía, no un error: la pantalla no debe gritar porque hoy
    // no ha entrado nada por la plataforma.
    retry: false,
  });
}
