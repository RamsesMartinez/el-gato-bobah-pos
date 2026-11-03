// presentationLabel es cómo se nombra una presentación en toda pantalla: con su marca, si tiene
// («Caja · Marca A»). Dos presentaciones del mismo insumo pueden llamarse igual con marca distinta
// (spec 056), así que el nombre solo ya no las distingue.
export function presentationLabel(p: { name: string; brand?: string | null }): string {
  const brand = p.brand?.trim();
  return brand ? `${p.name} · ${brand}` : p.name;
}
