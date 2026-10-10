import { useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';

// Trae el menú de nuevo y vuelve a pedir las cuentas abiertas con él.
//
// Hace falta cada vez que el catálogo cambia con cuentas ya armadas: alguien corrige un precio en
// ESTA tableta, o llega el evento `menu.updated` de otra. Desde la 030 el precio de cada renglón lo
// calcula el servidor en cada lectura de la cuenta, así que «repreciar» es volver a pedirla.
//
// El await no es de adorno: `invalidateQueries` espera al refetch de las queries activas.
export function useReprecio() {
  const qc = useQueryClient();
  return useCallback(async () => {
    await qc.invalidateQueries({ queryKey: ['menu'] });
    await qc.invalidateQueries({ queryKey: ['pos', 'draft'] });
  }, [qc]);
}
