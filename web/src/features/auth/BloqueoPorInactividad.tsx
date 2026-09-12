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
      {/* `inert` mientras está bloqueado, y no es cosmético: `LockScreen` se anuncia como
          `aria-modal`, y sin esto esa etiqueta es una promesa que el bloqueo no cumple. Con un
          teclado —las tabletas del local a veces traen uno— `Tab` desde el último botón del
          bloqueo caía en un control invisible de la pantalla de abajo y `Enter` lo activaba: usar
          el POS sin desbloquearlo, que es exactamente lo que esta pantalla existe para impedir.
          `inert` saca del foco, del tabulador y del alcance del puntero a todo el subárbol, así
          que la promesa la cumple el navegador y no una trampa de foco escrita a mano. */}
      {/* `display: contents` para que esta envoltura no exista para el layout: el POS encadena
          alturas al 100 % desde la raíz, y un `div` de verdad en medio le rompe el alto a la
          pantalla del mostrador. */}
      <div style={{ display: 'contents' }} inert={bloqueado ? true : undefined}>
        {children}
      </div>
      {bloqueado && <LockScreen onDesbloqueado={desbloquear} />}
    </>
  );
}
