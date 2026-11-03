import { ApiError } from '../../api/client';

// receiptError dice en palabras por qué no se guardó una acción de recibir (spec 038). Un 409 es
// otra tableta que recibió primero: se vuelve a abrir para ver lo que quedó.
export const receiptError = (e: unknown) => (e instanceof ApiError && e.status === 409
  ? 'Alguien más ya recibió en esta compra. Cierra y vuelve a abrir para ver lo que quedó.'
  : 'No se pudo guardar');
