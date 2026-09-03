import type { ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { posApi } from '../../api/pos';
import { useInactividad } from './useInactividad';
import { LockScreen } from './LockScreen';

// Envuelve la aplicación y le pone encima la pantalla de bloqueo cuando toca.
//
// ENCIMA y no en lugar de: los hijos siguen montados, así que lo capturado no se pierde. Si se
// perdiera, el operador aprendería a impedir que la tableta se bloquee —dejándola en movimiento, o
// pidiendo que se apague el ajuste— y con eso se cae toda la protección.
export function BloqueoPorInactividad({ children }: { children: ReactNode }) {
  const { data: ajustes } = useQuery({
    queryKey: ['business-settings'],
    queryFn: posApi.businessSettings,
    staleTime: 5 * 60 * 1000,
  });
  // Mientras los ajustes no llegan NO se bloquea. Bloquear con un default inventado dejaría la
  // tableta pidiendo PIN por una petición lenta, que es peor que tardar en proteger.
  const segundos = ajustes?.lockAfterSeconds ?? 0;
  const { bloqueado, desbloquear } = useInactividad(segundos, ajustes !== undefined);

  return (
    <>
      {children}
      {bloqueado && <LockScreen onDesbloqueado={desbloquear} />}
    </>
  );
}
