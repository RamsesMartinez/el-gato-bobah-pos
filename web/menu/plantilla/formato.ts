const entero = new Intl.NumberFormat('es-MX', { maximumFractionDigits: 0 });
const centavos = new Intl.NumberFormat('es-MX', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

/** «$45» sin centavos, «$45.50» con ellos, «$1,250» con miles. El precio llega ya redondeado. */
export function precio(p: string): string {
  const n = Number(p);
  if (!Number.isFinite(n)) return '';
  return '$' + (Number.isInteger(n) ? entero.format(n) : centavos.format(n));
}

/** La foto guardada es la de 1200; la de 400 se llama igual con -400 (spec 091). */
export function chica(url: string): string {
  return url.replace(/-1200\.webp$/, '-400.webp');
}
