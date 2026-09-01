// Cuándo se bloquea la pantalla por inactividad.
//
// La regla vive aparte del hook para tener test propio: es aritmética de tiempo, y los dos modos de
// fallar son caros y opuestos — bloquear a cada instante deja la tableta inservible, y no bloquear
// nunca deja la sesión abierta con el local vacío.

// vencio dice si ya pasó el tiempo sin actividad.
//
// Un tope de 0 o negativo significa "no bloquear". Cero es una elección válida —una caja en una
// oficina cerrada no lo necesita— y un negativo solo puede venir de datos corruptos: en los dos
// casos el modo de fallo tiene que dejar trabajar, no impedirlo.
export function vencio(ultimaActividad: number, ahora: number, segundos: number): boolean {
  if (segundos <= 0) return false;
  return ahora - ultimaActividad >= segundos * 1000;
}

// proximoVencimiento: en qué instante tocaría bloquear, o null si no se bloquea.
export function proximoVencimiento(ultimaActividad: number, segundos: number): number | null {
  if (segundos <= 0) return null;
  return ultimaActividad + segundos * 1000;
}
