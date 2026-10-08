import type { PaymentView } from '../../types/pos';

// MAX_PEOPLE es el espejo del tope de domain.MaxSplitParts. Solo decide hasta dónde sube el +; el
// servidor rechaza igual una serie más larga.
export const MAX_PEOPLE = 12;

// nextPart es la primera parte de la serie que nadie ha pagado. Las partes cobradas salen de los
// pagos del servidor: tras recargar la tableta, la que sigue sigue siendo la que sigue.
export function nextPart(of: number, payments: PaymentView[]): number | null {
  const charged = new Set(payments.filter((p) => !p.voided && p.split?.of === of).map((p) => p.split!.part));
  for (let part = 1; part <= of; part++) if (!charged.has(part)) return part;
  return null;
}

export const KEYS = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '.', '0', 'back'] as const;
export type Key = (typeof KEYS)[number];

// typeKey aplica una tecla al monto tecleado: a lo más dos decimales y un solo punto, para que lo
// que se ve sea lo que se cobra.
export function typeKey(value: string, key: Key): string {
  if (key === 'back') return value.slice(0, -1);
  if (key === '.') return value.includes('.') ? value : (value === '' ? '0.' : `${value}.`);
  const [, dec] = value.split('.');
  if (dec !== undefined && dec.length >= 2) return value;
  if (value === '0') return key;
  return value + key;
}

// VOID_REASONS son los motivos de devolver un pago. Sin preselección: el motivo queda en la bitácora
// y uno puesto por omisión se registraría sin que nadie lo haya elegido.
export const VOID_REASONS = [
  'Se le cobró a otra persona',
  'Se cobró con otro método',
  'Se cobró de más',
  'El cliente ya no lo paga',
] as const;
