import type { LowStockConfig } from '../../api/notifications';
import { formatBaseQty } from '../units';

// resumenDeAlertas dice en palabras qué avisa un artículo, para el renglón de la ficha y la confirmación.
export function resumenDeAlertas(c: Pick<LowStockConfig, 'enabled' | 'onMin' | 'onOut' | 'banner' | 'minStock' | 'unitCode'>): string {
  if (!c.enabled) return 'Apagadas';
  const partes = [];
  if (c.onMin && c.minStock && Number(c.minStock) > 0) partes.push(`al llegar a ${formatBaseQty(c.minStock, c.unitCode)}`);
  if (c.onOut) partes.push('al acabarse');
  const base = partes.length ? `Avisa ${partes.join(' y ')}` : 'Encendidas, sin evento';
  return c.banner ? `${base} · con franja` : base;
}

