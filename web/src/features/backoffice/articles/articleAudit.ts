import type { ArticleAuditItem } from '../../../api/backoffice';

const FIELD: Record<string, string> = { name: 'Nombre', minStock: 'Mínimo', cost: 'Costo', active: 'Activo' };

// whatChanged dice en una línea lo que cambió, solo con los campos que quien opera reconoce.
export function whatChanged(e: Pick<ArticleAuditItem, 'before' | 'after'>): string {
  const before = e.before ?? {};
  const after = e.after ?? {};
  const out: string[] = [];
  for (const [k, label] of Object.entries(FIELD)) {
    if (!(k in after)) continue;
    const show = (v: unknown) => (v === null || v === undefined || v === '' ? '—' : v === true ? 'sí' : v === false ? 'no' : String(v));
    out.push(k in before ? `${label}: ${show(before[k])} → ${show(after[k])}` : `${label}: ${show(after[k])}`);
  }
  return out.join(' · ');
}

