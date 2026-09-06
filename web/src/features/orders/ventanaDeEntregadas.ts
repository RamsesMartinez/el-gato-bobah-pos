// Cómo se llama la ventana de "entregadas" según lo que el negocio configuró.
//
// El rótulo decía "Entregadas hoy" siempre, aunque el negocio hubiera elegido que la lista se vacíe
// al abrir el siguiente turno o al cerrar la caja. Un encabezado que nombra un periodo distinto del
// que la lista contiene es la misma familia de defecto que ya costó un turno con $4,500 sin
// explicar: quien lo lee saca una conclusión sobre un periodo que nadie consultó.
//
// Vive aparte del componente y es pura a propósito: es una decisión, y una decisión se prueba.

export type CorteDeVista = 'medianoche' | 'turno' | 'cierre_de_caja';

/**
 * Título de la sección de entregadas para la ventana configurada.
 *
 * Un modo desconocido —un ajuste viejo, un valor metido por fuera— cae al de medianoche, que es el
 * default del producto. Nunca a un rótulo vacío ni al nombre interno del modo.
 */
export function tituloDeEntregadas(corte: string | undefined): string {
  switch (corte) {
    case 'turno':
      return 'Entregadas en este turno';
    case 'cierre_de_caja':
      return 'Entregadas desde el último corte';
    default:
      return 'Entregadas hoy';
  }
}

/** El vacío se redacta con la MISMA ventana que el título: si difieren, uno de los dos miente. */
export function vacioDeEntregadas(corte: string | undefined): string {
  switch (corte) {
    case 'turno':
      return 'Sin entregas en este turno';
    case 'cierre_de_caja':
      return 'Sin entregas desde el último corte';
    default:
      return 'Sin entregas hoy';
  }
}
