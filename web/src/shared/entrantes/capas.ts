// Las capas de lo que se pinta encima de la pantalla, en un solo lugar.
//
// La franja de pedidos por aceptar va ENCIMA DE TODO, incluido el bloqueo por PIN: la tableta se
// bloquea a los minutos sin tocarla, que es justo cuando suele llegar un pedido, y una alarma que
// suena sobre una pantalla que no dice por qué es una alarma que se apaga a manotazos.
export const CAPA_HOJAS = 1400; // modal de Chakra
export const CAPA_TOASTER = 1700; // toast de Chakra
export const CAPA_BLOQUEO = 2000; // LockScreen
export const CAPA_FRANJA = 2100;
