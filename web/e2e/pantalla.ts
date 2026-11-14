// La pantalla mínima contra la que se diseña y se mide (constitución, «Restricciones del producto»;
// decidido el 2026-11-13): el equipo de mostrador, 1280×854. Antes era la tableta de 1024×600; las
// pruebas que fijan ese tamaño a propósito vigilan el modo angosto, no el presupuesto.
export const ANCHO = 1280;
export const ALTO = 854;
// Con el teclado del sistema abierto queda poco más de la mitad del alto: se simula encogiendo la
// ventana, porque Chromium no pinta el teclado.
export const ALTO_CON_TECLADO = 470;
// 2K: ampliación. Se revisa que nada se estire ni se pierda, no se diseña para ella.
export const ANCHO_2K = 2560;
export const ALTO_2K = 1440;
