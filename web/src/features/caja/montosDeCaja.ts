import { moneyExact } from '../../utils/format';

// Menos de medio centavo es cero: el servidor ya redondea a 2 decimales, esto solo protege de un
// «-0.00».
export const CERO = 0.005;

// montoDeCaja: los montos de Caja van siempre con dos decimales (#290, FR-005), para que $1,200.00 y
// $85.50 se alineen en una columna. El resto del POS sigue con money().
export function montoDeCaja(v: string | number, currency: string): string {
  return moneyExact(v, currency);
}

// montoOSinDato: un importe que puede no viajar. Con el arqueo ciego el esperado llega en null —lo
// que la pantalla no debe mostrar no se le manda— y una raya es lo honesto: un cero diría que no se
// espera nada.
export function montoOSinDato(v: string | null, currency: string): string {
  return v === null ? '—' : montoDeCaja(v, currency);
}

// diffColor: el color de una diferencia. Nunca va solo (FR-004): lo acompaña <Diferencia>.
export function diffColor(v: string | number): string {
  const n = Number(v);
  if (n > CERO) return 'fg.overage';
  if (n < -CERO) return 'fg.shortage';
  return 'fg.muted';
}

