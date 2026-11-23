// La merma esperada (spec 096) viaja como número o null. Un campo vacío es «sin captura»: null, nunca
// 0 —un 0 capturado es una decisión («aquí no se pierde nada») y Number('') también da 0—.

export function pctTecleado(texto: string): number | null | 'invalido' {
  const t = texto.trim().replace('%', '').trim().replace(',', '.');
  if (t === '') return null;
  if (!/^\d{1,2}(\.\d{1,2})?$/.test(t)) return 'invalido';
  const n = Number(t);
  return n >= 0 && n < 100 ? n : 'invalido';
}

// pctTexto pone en el campo lo que llegó del servidor ("5.00" → "5"); nulo = vacío.
export function pctTexto(v: string | number | null | undefined): string {
  if (v === null || v === undefined || v === '') return '';
  return String(Number(v));
}
