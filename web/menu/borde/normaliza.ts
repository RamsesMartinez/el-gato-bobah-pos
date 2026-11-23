// La dirección que alguien escribe a mano («/Doña Lupe») se lleva al nombre normalizado. ESPEJO de
// domain.SlugifyMenuName (misma tabla de acentos); normaliza.test.ts lo compara contra la tabla de
// casos que comparten Go, la base y Ajustes. Es una copia a propósito: el menú no importa nada del POS.
const DE = 'áàâäãéèêëíìîïóòôöõúùûüñçÁÀÂÄÃÉÈÊËÍÌÎÏÓÒÔÖÕÚÙÛÜÑÇ';
const A = 'aaaaaeeeeiiiiooooouuuuncAAAAAEEEEIIIIOOOOOUUUUNC';

/** `recorta` = false para una dirección: recortarla podría llevar a un cliente al menú de otro negocio. */
export function normalizaNombre(s: string, recorta = true): string {
  let out = '';
  let guion = false;
  for (const ch of s) {
    const i = DE.indexOf(ch);
    let c = i >= 0 ? A[i] : ch;
    if (c >= 'A' && c <= 'Z') c = c.toLowerCase();
    if ((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
      out += c;
      guion = false;
    } else if (!guion && out.length > 0) {
      out += '-';
      guion = true;
    }
  }
  out = out.replace(/-+$/, '');
  if (recorta && out.length > 40) out = out.slice(0, 40).replace(/-+$/, '');
  return out;
}
