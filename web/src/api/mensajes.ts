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
  if (e instanceof ApiError) return MENSAJES_DE_CUENTA[e.code] ?? e.message;
  // Un fallo de red no tiene mensaje que sirva: el navegador dice "Failed to fetch" y quien opera
  // necesita saber qué hacer, no qué falló.
  if (e instanceof TypeError) return 'Revisa la conexión y vuelve a intentar.';
  return 'Vuelve a intentar. Si sigue, recarga la pantalla.';
}

// Los rechazos de la cuenta en captura (spec 030), escritos para quien opera. Van por código y no
// por el texto del servidor: es la misma frase en las tres pantallas que los pueden recibir, y
// ninguna nombra cómo está hecho por dentro.
const MENSAJES_DE_CUENTA: Record<string, string> = {
  DRAFT_CHANGED: 'La cuenta cambió en otra tableta. Ya se muestra como quedó.',
  DRAFT_DISCARDED: 'Esa cuenta se descartó en otra tableta.',
  DRAFT_SENT: 'Esa cuenta ya se mandó a cocina. Para quitarla hay que cancelar el pedido.',
  ORDER_CLOSED: 'Esa cuenta ya está pagada y entregada. Lo que pidan ahora va en una cuenta nueva.',
  PLATFORM_ORDER_NO_LINES: 'A un pedido de plataforma no se le agregan productos.',
};
