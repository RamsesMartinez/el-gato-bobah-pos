import { useQuery } from '@tanstack/react-query';
import { posApi } from '../../api/pos';
import { reportarResultado } from './useSinConexion';

// Todas las cuentas vivas de la empresa: las que se capturan en cualquier tableta y los pedidos
// que no se han cerrado. Es LA fila del POS.
//
// Cada 30 s y con cada evento del servidor (`useOrderEvents`). El intervalo no sobra: el canal de
// eventos se cae en silencio en una tableta suspendida, y al despertar la fila tiene que ponerse al
// día sin que nadie toque nada. También es el latido que apaga el aviso de sin conexión en cuanto
// el servidor vuelve a contestar.
export function useCuentasVivas() {
  return useQuery({
    queryKey: ['pos', 'accounts'],
    queryFn: async () => {
      try {
        const r = await posApi.liveAccounts();
        reportarResultado(null);
        return r;
      } catch (e) {
        reportarResultado(e);
        throw e;
      }
    },
    refetchInterval: 30_000,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
  });
}
