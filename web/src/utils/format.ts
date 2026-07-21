// El backend envía dinero como string decimal exacto ("70.50"); el front lo parsea solo
// para FORMATEAR (el servidor es la fuente de verdad del cálculo). Acepta string o number.
// currency (ISO-4217) elige el símbolo; el locale se queda en es-MX. Sin centavos se ocultan
// los ".00" ($70, no $70.00); con centavos se muestran ($70.50) para no engañar.
export function money(v: string | number, currency: string = 'MXN'): string {
  const n = typeof v === 'string' ? Number(v) : v;
  return n.toLocaleString('es-MX', {
    style: 'currency', currency,
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
