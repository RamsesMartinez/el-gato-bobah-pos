import type { QueryClient } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { posApi } from '../../api/pos';
import { toaster } from '../../components/ui/toaster';
import { accionPropia } from '../../stores/accionesPropias';

// descartarCuenta descarta con la versión que la tableta vio y dice si se descartó.
//
// Descartar se lleva todo lo de adentro. Si otra tableta agregó o cambió algo entretanto, el servidor
// responde DRAFT_CHANGED sin tocar nada: aquí se recarga la cuenta para que se vea como quedó y se
// avisa, en vez de darla por descartada. Cualquier otro rechazo lo maneja quien llama.
export async function descartarCuenta(qc: QueryClient, id: string, version: number): Promise<boolean> {
  try {
    await accionPropia(() => posApi.discardDraft(id, version));
    return true;
  } catch (e) {
    if (!(e instanceof ApiError && e.code === 'DRAFT_CHANGED')) throw e;
    qc.invalidateQueries({ queryKey: ['pos', 'draft', id] });
    qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
    toaster.create({
      title: 'La cuenta cambió en otra tableta',
      description: 'No se descartó. Ya se muestra como quedó.',
      type: 'info',
    });
    return false;
  }
}
