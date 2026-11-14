// Los motivos son los mismos que acepta el servidor (domain.ValidOpeningReason).
export const MOTIVOS_DE_APERTURA: { value: string; label: string }[] = [
  { value: 'last_count_wrong', label: 'El cierre se contó mal' },
  { value: 'float_changed', label: 'Cambié el fondo' },
  { value: 'unrecorded_withdrawal', label: 'Se sacó dinero sin registrar' },
  { value: 'other', label: 'Otro' },
];

// rotuloDeMotivo: el motivo como lo eligió quien abrió; el valor crudo solo si el servidor manda uno
// que esta pantalla todavía no conoce.
export function rotuloDeMotivo(value: string): string {
  return MOTIVOS_DE_APERTURA.find((m) => m.value === value)?.label ?? value;
}

