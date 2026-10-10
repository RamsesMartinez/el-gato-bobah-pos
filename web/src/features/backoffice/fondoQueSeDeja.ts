// fondoValido: el fondo que se deja al cerrar. Un campo vacío no es cero (`Number('')` sí lo es):
// falta capturarlo.
export function fondoValido(v: string): boolean {
  const t = v.trim();
  const n = Number(t);
  return t !== '' && Number.isFinite(n) && n >= 0;
}
