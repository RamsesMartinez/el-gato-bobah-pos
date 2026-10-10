import type { CardTerminal } from '../api/backoffice';

// terminalEfectiva decide con qué terminal se cobra: la elegida en esta hoja, si no la del usuario,
// si no la única activa. Archivadas nunca. `null` = hay que elegir. Es la misma regla del servidor
// (resolveTerminal), para que la hoja no deje cobrar algo que va a rebotar.
export function terminalEfectiva(terminales: CardTerminal[], porOmision: number | null, elegida: number | null): number | null {
  const activas = terminales.filter((t) => !t.archived);
  const existe = (id: number | null) => id !== null && activas.some((t) => t.id === id);
  if (existe(elegida)) return elegida;
  if (existe(porOmision)) return porOmision;
  return activas.length === 1 ? activas[0].id : null;
}
