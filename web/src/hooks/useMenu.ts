import { useQuery } from '@tanstack/react-query';
import { posApi } from '../api/pos';

// Un solo payload con todo el catálogo (categorías + productos + modificadores), cacheado en
// memoria. Sin fetches por tap.
//
// Se revisa cada minuto y al volver al frente (el foco de React Query escucha `visibilitychange`):
// el aviso en vivo `menu.updated` no llega si la tableta estaba suspendida o la red parpadeó, y el
// 2026-10-10 eso dejó a la tableta ofreciendo un extra que ya no existía. La revisión es
// condicional (304 sin catálogo), así que cada minuto cuesta un request vacío. Si falla, React
// Query conserva el menú que había: sin red se sigue vendiendo con lo último.
export function useMenu() {
  return useQuery({
    queryKey: ['menu'],
    queryFn: posApi.menu,
    staleTime: 30_000,
    refetchInterval: 60_000,
    refetchOnWindowFocus: 'always',
    gcTime: Infinity,
  });
}
