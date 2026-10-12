import { money } from '../../utils/format';

// diferenciaDelHistorico: lo que dice la columna «Diferencia» del histórico. Oculto no es cero.
export function diferenciaDelHistorico(r: { status: string; totalDifference: string | null; arqueoHidden?: boolean; currency: string }): string {
  if (r.status !== 'cerrada') return '—';
  if (r.arqueoHidden || r.totalDifference === null) return 'Oculto';
  return money(r.totalDifference, r.currency);
}
