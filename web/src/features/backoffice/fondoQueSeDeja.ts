import { money } from '../../utils/format';
import { round2 } from '../../domain/cobro';

// fondoValido: el fondo que se deja al cerrar. Un campo vacío no es cero (`Number('')` sí lo es):
// falta capturarlo.
export function fondoValido(v: string): boolean {
  const t = v.trim();
  const n = Number(t);
  return t !== '' && Number.isFinite(n) && n >= 0;
}

// fondoExcedeLoContado: el fondo no puede ser mayor a lo que hay en el cajón (EB-48). Sin conteo
// todavía no hay contra qué comparar.
export function fondoExcedeLoContado(fondo: string, contado: number | null): boolean {
  return contado !== null && fondoValido(fondo) && Number(fondo.trim()) > contado;
}

// retiroDelCierre: lo contado menos el fondo que se deja (spec 035). Nulo mientras falte una de las
// dos cifras o el fondo pase de lo contado: no hay retiro que mostrar. Son dos cifras que captura el
// operador, así que mostrarlo no revela el esperado del arqueo ciego.
export function retiroDelCierre(contado: number | null, fondo: string): number | null {
  if (contado === null || !fondoValido(fondo) || fondoExcedeLoContado(fondo, contado)) return null;
  return round2(contado - Number(fondo.trim()));
}

// personasDelRetiro: quien cierra va primero, que es quien casi siempre se lleva el dinero.
export function personasDelRetiro<T extends { id: number }>(personas: T[], quienCierra: number | null): T[] {
  const yo = personas.filter((p) => p.id === quienCierra);
  return [...yo, ...personas.filter((p) => p.id !== quienCierra)];
}

// resumenDelRetiro: lo que dice la hoja de «¿Cerrar caja?». La persona viene marcada por omisión y
// el retiro no se corrige después, así que se lee justo antes del toque que lo vuelve definitivo.
export function resumenDelRetiro({ retirar, retiro, quien, contado, currency }: {
  retirar: boolean; retiro: number | null; quien: string | null; contado: number | null; currency: string;
}): string {
  const fin = 'No podrás modificarla después.';
  if (contado === null) return fin;
  if (!retirar || retiro === 0) return `Todo se queda en el cajón: ${money(contado, currency)}. ${fin}`;
  if (retiro === null) return fin;
  return `Se retiran ${money(retiro, currency)} · se los lleva ${quien ?? 'nadie'}. ${fin}`;
}

// avisoDeQuienSeLoLleva: con retiro y sin persona elegida, «Cerrar caja» no se activa; esto dice qué
// tocar. Va aparte de «Falta capturar lo contado»: elegir a alguien no es contar nada.
export function avisoDeQuienSeLoLleva(faltaQuien: boolean, retiro: number | null, currency: string): string | null {
  if (!faltaQuien || retiro === null) return null;
  return `Elige quién se lleva el retiro de ${money(retiro, currency)}`;
}
