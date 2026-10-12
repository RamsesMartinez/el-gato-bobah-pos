import type { CardTerminal } from '../api/backoffice';

// terminalEfectiva decide con qué terminal se cobra: la elegida en esta hoja, si no la única activa.
// Archivadas nunca. `null` = hay que elegir. Es la misma regla del servidor (resolveTerminal), para
// que la hoja no deje cobrar algo que va a rebotar. Ya no hay terminal del usuario (spec 036).
export function terminalEfectiva(terminales: CardTerminal[], elegida: number | null): number | null {
  const activas = terminales.filter((t) => !t.archived);
  if (elegida !== null && activas.some((t) => t.id === elegida)) return elegida;
  return activas.length === 1 ? activas[0].id : null;
}
