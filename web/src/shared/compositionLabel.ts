import type { CompositionStatus } from '../api/admin';

// compositionLabel es el estado en una palabra, para el botón que abre la receta.
export function compositionLabel(status: CompositionStatus | undefined): string {
  if (status === 'estimated') return 'Por revisar';
  if (status === 'confirmed') return 'Lista';
  return 'Pendiente';
}
