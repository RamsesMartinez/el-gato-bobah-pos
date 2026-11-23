import { useQuery } from '@tanstack/react-query';
import { posApi } from '../api/pos';

// Los productos que salen «Agotado» (spec 092). Se revisan cada minuto y al volver al frente, como
// el menú; si la revisión falla se conserva la última lista. Una respuesta sin el arreglo (una API
// anterior) se lee como «nada agotado».
export function useSoldOut(): Set<number> {
  const { data } = useQuery({
    queryKey: ['menu', 'sold-out'],
    queryFn: async () => new Set((await posApi.soldOut()).productIds ?? []),
    staleTime: 30_000,
    refetchInterval: 60_000,
    refetchOnWindowFocus: 'always',
  });
  return data ?? VACIO;
}

const VACIO = new Set<number>();
