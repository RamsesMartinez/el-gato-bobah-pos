// La vista previa del nombre del menú (spec 097). ESPEJO de domain.SlugifyMenuName y de
// public_menu_slugify: misma tabla de acentos y mismos pasos. slug.test.ts lo compara contra la tabla
// de casos que también usan Go y la base.
const DE = 'áàâäãéèêëíìîïóòôöõúùûüñçÁÀÂÄÃÉÈÊËÍÌÎÏÓÒÔÖÕÚÙÛÜÑÇ';
const A = 'aaaaaeeeeiiiiooooouuuuncAAAAAEEEEIIIIOOOOOUUUUNC';

export function slugDelMenu(s: string): string {
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
  if (out.length > 40) out = out.slice(0, 40).replace(/-+$/, '');
  return out;
}
