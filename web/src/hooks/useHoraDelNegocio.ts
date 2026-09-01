import { useQuery } from '@tanstack/react-query';
import { posApi } from '../api/pos';
import { DEFAULT_TIMEZONE } from '../utils/zonaPorDefecto';
import { fechaYHora, soloFecha, soloHora, zonaEsUsable, zonaSegura } from '../utils/horaDelNegocio';

// La zona del negocio, para las pantallas.
//
// Es el ÚNICO lugar del que las pantallas sacan cómo formatear una hora. Antes cada una llamaba a
// `toLocaleString` por su cuenta y sin zona, así que todas decían la hora del navegador de esa
// tableta; once sitios sueltos es cómo eso se desincronizó, y que haya uno solo es lo que impide
// que vuelva.
//
// `lista` existe para que nadie pinte una hora antes de conocer la zona. Pintar con el default y
// corregir después hace que el operador vea la hora saltar, y una hora que salta es una hora en la
// que se deja de confiar.
export function useHoraDelNegocio() {
  const { data, isPending } = useQuery({
    queryKey: ['business-settings'],
    queryFn: posApi.businessSettings,
    // La zona de un negocio no cambia entre pantallas: se pide una vez y se reusa. Sin esto, cada
    // pantalla que muestre una hora dispara su propia petición.
    staleTime: 5 * 60_000,
  });

  const guardada = data?.timezone;
  const zona = zonaSegura(guardada);
  // Una zona guardada que el navegador no reconoce se usa igual —cae al default— pero deja
  // constancia: sin esto el negocio se comporta bien, muestra la hora de otro lado, y nadie se
  // entera nunca.
  const zonaRota = guardada !== undefined && guardada !== '' && !zonaEsUsable(guardada);

  return {
    zona,
    esElDefault: zona === DEFAULT_TIMEZONE && guardada !== DEFAULT_TIMEZONE,
    zonaRota,
    zonaGuardada: guardada,
    lista: !isPending,
    fechaYHora: (v: string | Date | null | undefined) => fechaYHora(v, zona),
    soloHora: (v: string | Date | null | undefined) => soloHora(v, zona),
    soloFecha: (v: string | Date | null | undefined) => soloFecha(v, zona),
  };
}
