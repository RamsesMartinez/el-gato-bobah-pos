import { ApiError } from '../../api/client';
import { isOffline } from '../../api/mensajes';

// Por qué no se pudo checar, y qué se le dice a quien checa. Lo comparten la pantalla de Asistencia
// y la hoja de PIN de la caja (spec 046): checar desde la caja es el mismo registro, con el mismo
// bloqueo por PIN, y tiene que decirse igual.
export type ClockErrorKind = 'too_many' | 'state_changed' | 'offline' | 'no_match';

export function clockError(e: unknown): { kind: ClockErrorKind; message: string } {
  if (e instanceof ApiError && e.status === 429) {
    const min = Math.max(1, Math.ceil((e.retryAfter ?? 60) / 60));
    return { kind: 'too_many', message: `Demasiados intentos. Intenta de nuevo en ${min} min.` };
  }
  if (e instanceof ApiError && e.code === 'ATTENDANCE_STATE_CHANGED') {
    return { kind: 'state_changed', message: 'Tu registro cambió en otra tableta. Vuelve a tocar tu nombre.' };
  }
  if (isOffline(e)) return { kind: 'offline', message: 'Sin conexión. Checa cuando vuelva la red.' };
  // No dice si falló la persona o el PIN, igual que el bloqueo: distinguirlos enumera usuarios.
  return { kind: 'no_match', message: 'No coincide. Inténtalo de nuevo.' };
}
