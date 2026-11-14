// Los motivos de una corrección son los mismos que acepta el servidor (domain.ValidateAdjustment).
export const MOTIVOS_DE_CORRECCION: { value: string; label: string }[] = [
  { value: 'miscount', label: 'Se contó mal' },
  { value: 'unrecorded_movement', label: 'Movimiento sin registrar' },
  { value: 'wrong_register', label: 'Salió de otra caja' },
  { value: 'float_error', label: 'Error en el fondo' },
  { value: 'other', label: 'Otro' },
];

export function rotuloDeCorreccion(value: string): string {
  return MOTIVOS_DE_CORRECCION.find((m) => m.value === value)?.label ?? value;
}
