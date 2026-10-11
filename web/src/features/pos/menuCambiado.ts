import { ApiError } from '../../api/client';
import { toaster } from '../../components/ui/toaster';

// El rechazo por menú viejo lleva su propio código (MENU_CHANGED) para no adivinarlo del texto.
export function esMenuViejo(e: unknown): e is ApiError {
  return e instanceof ApiError && e.code === 'MENU_CHANGED';
}

// Aviso neutro con id fijo: varios toques rechazados seguidos por lo mismo dejan UN aviso, no una
// pila. No es un error del operador, así que no va en rojo.
export function avisarMenuCambiado() {
  toaster.create({ id: 'menu-cambiado', title: 'El menú se actualizó; elige de nuevo', type: 'info' });
}
