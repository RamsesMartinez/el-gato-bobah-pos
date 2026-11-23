import type { CostLine, ProductCosting, WasteSource } from '../../api/admin';

// De dónde vino el % de cada renglón, en palabras de quien opera (spec 096).
export const ORIGEN: Record<WasteSource, string> = {
  article: 'de este artículo',
  product: 'de este producto',
  general: 'general del negocio',
  none: 'sin merma',
};

export const r4 = (x: number) => Math.round(x * 1e4) / 1e4;

// conMermaDelProducto es la vista previa del desglose con el % que se está tecleando: los renglones
// que no tienen % propio del artículo toman el del producto o, vacío, el general. La suma sale de los
// renglones ya redondeados, igual que en el servidor (SC-003).
export function conMermaDelProducto(c: ProductCosting, typed: number | null): { lines: CostLine[]; base: number; waste: number } {
  const lines = (c.lines ?? []).map((l) => {
    if (l.pctSource === 'article') return l;
    const pct = typed ?? c.generalPct ?? 0;
    const pctSource = typed !== null ? 'product' : c.generalPct !== null ? 'general' : 'none';
    return { ...l, pct, pctSource, waste: String(r4((Number(l.base) * pct) / 100)) } as CostLine;
  });
  const base = lines.reduce((s, l) => s + Number(l.base), 0);
  const waste = lines.reduce((s, l) => s + Number(l.waste), 0);
  return { lines, base: r4(base), waste: r4(waste) };
}

