// fondoValido: el fondo que se deja al cerrar. Un campo vacío no es cero (`Number('')` sí lo es):
// falta capturarlo.
export function fondoValido(v: string): boolean {
  const t = v.trim();
  const n = Number(t);
  return t !== '' && Number.isFinite(n) && n >= 0;
}

// fondoExcedeLoContado: el fondo no puede ser mayor a lo que hay en el cajón (EB-48). Sin conteo
// todavía no hay contra qué comparar.
export function fondoExcedeLoContado(fondo: string, contado: number | null): boolean {
  return contado !== null && fondoValido(fondo) && Number(fondo.trim()) > contado;
}
