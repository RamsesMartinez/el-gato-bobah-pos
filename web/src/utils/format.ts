// Sin productos con centavos: se ocultan los ".00" ($70, no $70.00). Si por alguna
// razón hay centavos (cambio en efectivo), se muestran ($70.50) para no engañar.
export function money(n: number): string {
  return n.toLocaleString('es-MX', {
    style: 'currency', currency: 'MXN',
    minimumFractionDigits: 0, maximumFractionDigits: 2,
  });
}

// Hash estable → hue, para colorear categorías consistentemente entre sesiones.
export function categoryColor(id: number, override?: string | null): string {
  if (override) return override;
  let h = 2166136261;
  const s = String(id);
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  const hue = Math.abs(h) % 360;
  return `hsl(${hue}, 62%, 52%)`;
}

// Quita acentos para búsqueda insensible a diacríticos (rango de marcas combinantes).
export function normalize(s: string): string {
  return s
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase();
}
