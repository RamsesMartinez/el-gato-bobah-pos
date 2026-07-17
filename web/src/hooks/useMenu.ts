import { useQuery } from '@tanstack/react-query';
import { posApi } from '../api/pos';

// Un solo payload con todo el catálogo (categorías + productos + modificadores),
// cacheado en memoria. Sin fetches por tap.
export function useMenu() {
  return useQuery({
    queryKey: ['menu'],
    queryFn: posApi.menu,
    staleTime: 5 * 60 * 1000,
    gcTime: Infinity,
  });
}
