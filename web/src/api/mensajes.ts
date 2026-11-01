import { ApiError } from './client';

// El mensaje del servidor, listo para una pantalla.
//
// `String(e)` pinta el objeto crudo: "TypeError: Failed to fetch" cuando se cae la red, y "Error: "
// pegado delante de cada rechazo del servidor. La hoja de cobro ya lo tenía resuelto para su caso;
// el tablero de pedidos seguía con el objeto crudo en entregar, cancelar y reembolsar.
//
// La constitución lo prohíbe en dos frentes: en pantalla no van internals, y un aviso que el
// operador no puede accionar es peor que ninguno.
// isOffline: el pedido no llegó al servidor. Un ApiError con otro código es que el servidor sí
// respondió, y mandar a revisar el internet sería mandar a buscar donde no es.
export function isOffline(e: unknown): boolean {
  // Solo NETWORK: el cliente ya envuelve el fallo de `fetch`. Un TypeError suelto es un defecto del
  // código, y mandar a revisar el wifi por eso es mandar a buscar donde no es.
  return e instanceof ApiError && e.code === 'NETWORK';
}

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
  // Caja con asistencia (spec 046). OPENER_NOT_CLOCKED_OUT no va aquí: el del servidor nombra a la
  // persona («Ana abrió esta caja…»), y una frase fija la perdería.
  CLOCK_IN_REQUIRED: 'Para abrir la caja, primero marca tu entrada.',
  // «En el arqueo» de un pago ya guardado (spec 048).
  PAYMENT_IN_CLOSED_COUNT: 'El corte de ese pago ya se cerró: ya no se puede cambiar.',
  EXPENSE_BEFORE_LAST_CLOSE: 'La caja ya se cerró después de este pago: no puede entrar a ningún corte.',
  CLOCK_OUT_WITH_PIN: 'Marca tu salida con tu PIN para cerrar la caja.',
  // Al cerrar: otro cierre autorizado ya dejó la entrada «sin salida». Al reintentar, cierra.
  ATTENDANCE_STATE_CHANGED: 'La asistencia cambió en otra tableta. Vuelve a intentarlo.',
};
