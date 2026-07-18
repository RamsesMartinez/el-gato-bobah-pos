import { useQuery } from '@tanstack/react-query';
import { posApi } from '../api/pos';

// Defaults contextuales de modificadores (producto→grupo→opciones rankeadas).
// El backend recomputa por bucket de hora; el cliente refetchea cada 15 min (y al
// re-enfocar) para no quedarse pegado al primer snapshot en un POS abierto todo el día.
// Si falla, el modal usa su fallback.
export function useModifierDefaults() {
  return useQuery({
    queryKey: ['modifier-defaults'],
    queryFn: posApi.modifierDefaults,
    staleTime: 10 * 60 * 1000,
    refetchInterval: 15 * 60 * 1000,
  });
}
