import { useQuery } from '@tanstack/react-query';
import { posApi } from '../api/pos';

// Defaults contextuales de modificadores (producto→grupo→opciones rankeadas).
// Cambian por hora del día → staleTime corto. Si falla, el modal usa su fallback.
export function useModifierDefaults() {
  return useQuery({
    queryKey: ['modifier-defaults'],
    queryFn: posApi.modifierDefaults,
    staleTime: 10 * 60 * 1000,
  });
}
