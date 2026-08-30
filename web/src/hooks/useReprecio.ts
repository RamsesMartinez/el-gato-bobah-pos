import { useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';

import { repreciador } from '../features/pos/precioPlataforma';
import { useTicketStore } from '../stores/ticket';
import type { Menu } from '../types/pos';

// Trae el menú de nuevo y vuelve a precisar las cuentas abiertas con él.
//
// Hace falta cada vez que el catálogo cambia con tickets ya armados, porque el precio se congela en
// la línea al agregarla y el servidor recalcula al cobrar: sin esto la pantalla muestra un total y
// se cobra otro. Pasa por dos caminos —alguien corrige un precio en ESTA tablet, o llega el evento
// `menu.updated` de otra— y los dos necesitan lo mismo, así que vive aquí y no duplicado.
//
// El await no es de adorno: `invalidateQueries` espera al refetch de las queries activas, y
// re-precisar antes de que llegue el menú nuevo volvería a poner los precios viejos.
export function useReprecio() {
  const qc = useQueryClient();
  const repreciarTodas = useTicketStore((s) => s.repreciarTodas);

  return useCallback(async () => {
    await qc.invalidateQueries({ queryKey: ['menu'] });
    const menu = qc.getQueryData<Menu>(['menu']);
    repreciarTodas((tab) => repreciador(menu, tab.platformId));
  }, [qc, repreciarTodas]);
}
