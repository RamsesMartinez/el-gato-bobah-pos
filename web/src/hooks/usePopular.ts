import { useQuery } from '@tanstack/react-query';
import { posApi } from '../api/pos';

// IDs más vendidos para la pestaña "Top". Servidor cachea 5 min; el cliente refetch
// cada 5 min (y al re-enfocar), así un POS abierto todo el día ve el Top al día sin cron.
export function usePopular() {
  return useQuery({
    queryKey: ['popular'],
    queryFn: () => posApi.popular().then((r) => r.items),
    staleTime: 5 * 60 * 1000,
    refetchInterval: 5 * 60 * 1000,
  });
}
