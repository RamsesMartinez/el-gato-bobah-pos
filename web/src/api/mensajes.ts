import { ApiError } from './client';

// El mensaje del servidor, listo para una pantalla.
//
// `String(e)` pinta el objeto crudo: "TypeError: Failed to fetch" cuando se cae la red, y "Error: "
// pegado delante de cada rechazo del servidor. La hoja de cobro ya lo tenía resuelto para su caso;
// el tablero de pedidos seguía con el objeto crudo en entregar, cancelar y reembolsar.
//
// La constitución lo prohíbe en dos frentes: en pantalla no van internals, y un aviso que el
// operador no puede accionar es peor que ninguno.
export function mensajeDeError(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  // Un fallo de red no tiene mensaje que sirva: el navegador dice "Failed to fetch" y quien opera
  // necesita saber qué hacer, no qué falló.
  if (e instanceof TypeError) return 'Revisa la conexión y vuelve a intentar.';
  return 'Vuelve a intentar. Si sigue, recarga la pantalla.';
}
