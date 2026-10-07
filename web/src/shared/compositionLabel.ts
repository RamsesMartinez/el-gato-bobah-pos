import type { CompositionStatus } from '../api/admin';

// compositionLabel es el estado en una palabra, para el botón que abre «Qué lleva».
export function compositionLabel(status: CompositionStatus | undefined): string {
  if (status === 'estimated') return 'Estimado';
  if (status === 'confirmed') return 'Confirmado';
  return 'Sin capturar';
}
