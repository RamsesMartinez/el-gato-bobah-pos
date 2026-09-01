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

// Dónde se recuerda que la pantalla quedó bloqueada.
//
// El estado del bloqueo NO puede vivir solo en React: al recargar, el árbol arranca de cero con
// `bloqueado = false` y el canje del refresh devuelve la sesión completa del operador anterior.
// O sea que la protección entera se saltaba con la tecla de recargar, y todo lo que se cobrara
// después quedaba a nombre de quien dejó la tableta.
const CLAVE = 'egb:bloqueada';

// Almacen: lo mínimo de sessionStorage que hace falta. Se recibe como parámetro para poder probar
// qué pasa cuando el navegador lo tiene deshabilitado, que no es un caso hipotético — pasa en
// ventana privada y con políticas de empresa.
export interface Almacen {
  getItem(k: string): string | null;
  setItem(k: string, v: string): void;
  removeItem(k: string): void;
}

export function marcarBloqueada(a: Almacen): void {
  try {
    a.setItem(CLAVE, '1');
  } catch {
    // Sin dónde escribir, el bloqueo sigue valiendo en memoria durante esta carga. Reventar aquí
    // tumbaría la aplicación por no poder guardar una bandera.
  }
}

export function limpiarBloqueo(a: Almacen): void {
  try {
    a.removeItem(CLAVE);
  } catch { /* ídem */ }
}

// estabaBloqueada decide si hay que arrancar bloqueado.
//
// Si el almacén truena al leer, se asume QUE SÍ. Es una protección: una que se cae sola ante un
// error del navegador no protege de nada, y el costo de equivocarse hacia este lado es un PIN de
// más — contra una sesión ajena abierta si se equivoca hacia el otro.
export function estabaBloqueada(a: Almacen): boolean {
  try {
    return a.getItem(CLAVE) !== null;
  } catch {
    return true;
  }
}
