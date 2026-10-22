import type { ArticleAuditItem } from '../../../api/backoffice';
import { formatBaseQty, formatUnitCost } from '../../../shared/units';

const FIELD: Record<string, string> = { name: 'Nombre', minStock: 'Mínimo', cost: 'Costo', active: 'Activo' };

// whatChanged dice en una línea lo que cambió, solo con los campos que quien opera reconoce. El costo y
// el mínimo se guardan por la unidad base (precio por ml); se dicen como se compran («$24.00 / L»).
export function whatChanged(e: Pick<ArticleAuditItem, 'before' | 'after'>, baseUnitCode: string): string {
  const before = e.before ?? {};
  const after = e.after ?? {};
  const out: string[] = [];
  const show = (k: string, v: unknown) => {
    if (v === null || v === undefined || v === '') return '—';
    if (v === true) return 'sí';
    if (v === false) return 'no';
    if (k === 'cost') return formatUnitCost(String(v), baseUnitCode) ?? 'Sin costo';
    if (k === 'minStock') return formatBaseQty(String(v), baseUnitCode);
    return String(v);
  };
  for (const [k, label] of Object.entries(FIELD)) {
    if (!(k in after)) continue;
    out.push(k in before ? `${label}: ${show(k, before[k])} → ${show(k, after[k])}` : `${label}: ${show(k, after[k])}`);
  }
  return out.join(' · ');
}
