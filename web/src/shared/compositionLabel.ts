import type { CompositionStatus } from '../api/admin';
import type { Unit } from '../api/backoffice';

// compositionLabel es el estado en una palabra, para el botón que abre la receta.
export function compositionLabel(status: CompositionStatus | undefined): string {
  if (status === 'estimated') return 'Por revisar';
  if (status === 'confirmed') return 'Lista';
  return 'Pendiente';
}

const SMALLER: Record<string, string> = { kg: 'g', l: 'ml' };

// friendlyQuantity muestra en gramos o mililitros lo que no llega a un kilo o un litro: FUDO guarda
// en la unidad grande y «0.215 kg» se lee mal y se corrige peor que «215 g».
export function friendlyQuantity(q: number, unitId: number, units: Unit[]): { text: string; unitId: number } {
  const from = units.find((u) => u.id === unitId);
  const to = from && units.find((u) => u.code === SMALLER[from.code] && u.kind === from.kind);
  if (!from || !to || !(q > 0 && q < 1)) return { text: String(Number(q.toFixed(4))), unitId };
  return { text: String(Number(((q * Number(from.toBase)) / Number(to.toBase)).toFixed(4))), unitId: to.id };
}
