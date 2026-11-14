import type { AperturaInput } from '../../api/backoffice';
import type { ResultadoDelConteo } from './conteo';
import { soloHora } from '../../utils/horaDelNegocio';

// TOQUE_44: la zona de un interruptor de tabla, del tamaño que un dedo acierta a la primera.
export const TOQUE_44 = { minH: '44px', minW: '44px', display: 'inline-flex', alignItems: 'center', justifyContent: 'center' } as const;

// aperturaDelConteo traduce lo que entrega la hoja al cuerpo que espera el endpoint.
//
// La hoja no habla de `openingCash` a propósito: la misma sirve para abrir y para cerrar, y cada
// uno manda el efectivo en un campo distinto. La unión discriminada es la que impide mandar los dos
// caminos juntos, que es un 400 con el cajón ya contado.
export function aperturaDelConteo(r: ResultadoDelConteo): AperturaInput {
  return 'counts' in r ? { counts: r.counts } : { openingCash: r.total, manualReason: r.manualReason };
}

// La zona llega como parámetro: esta es una función de módulo.
export function hhmm(iso: string, zona: string) {
  return soloHora(iso, zona);
}
