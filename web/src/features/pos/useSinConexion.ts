import { useSyncExternalStore } from 'react';
import { ApiError } from '../../api/client';

// ¿Hay conexión con el servidor? (spec 030, D-4: capturar exige red).
//
// Son DOS señales y basta una: el navegador dice que no hay red, o la última petición del POS no
// llegó. La segunda existe porque el wifi del local se cae «hacia adentro»: el navegador sigue
// diciendo `onLine` —hay red local— y ninguna petición sale. Fiarse solo de `navigator.onLine`
// dejaría «Cobrar» encendido sin servidor.
//
// Vuelve sola: la primera respuesta del servidor —éxito o rechazo, un rechazo también prueba que
// hay conexión— apaga el aviso.

let falloDeRed = false;
const oyentes = new Set<() => void>();
const avisar = () => oyentes.forEach((o) => o());

export function esFalloDeRed(e: unknown): boolean {
  if (e instanceof ApiError) return e.code === 'NETWORK';
  return e instanceof TypeError;
}

// reportarResultado lo llama toda petición de la cuenta al terminar: `null` es éxito.
export function reportarResultado(e: unknown | null): void {
  const ahora = e !== null && esFalloDeRed(e);
  if (ahora === falloDeRed) return;
  falloDeRed = ahora;
  avisar();
}

function suscribir(o: () => void) {
  oyentes.add(o);
  window.addEventListener('online', o);
  window.addEventListener('offline', o);
  return () => {
    oyentes.delete(o);
    window.removeEventListener('online', o);
    window.removeEventListener('offline', o);
  };
}

const foto = () => falloDeRed || !navigator.onLine;

export function useSinConexion(): boolean {
  return useSyncExternalStore(suscribir, foto, () => false);
}
