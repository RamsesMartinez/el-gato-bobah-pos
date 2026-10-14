import { parseNumero, round2 } from '../../domain/numeros';
import { formatBaseQty } from '../../shared/units';

// Reglas puras de la captura del conteo (spec 038). Se cuenta como se ve en el anaquel —«3
// botellas + 0.5 L abiertos»— y al servidor llega la cifra en la unidad base.

// looseUnitOf es la unidad en la que se cuenta lo suelto o abierto de un insumo: la grande de su
// tipo, que es como lo dice alguien («0.5 L», no «500 ml»).
export function looseUnitOf(baseCode: string): { label: string; factor: number; decimals: boolean } {
  switch (baseCode) {
    case 'g': return { label: 'kg', factor: 1000, decimals: true };
    case 'ml': return { label: 'L', factor: 1000, decimals: true };
    case 'cm': return { label: 'm', factor: 100, decimals: true };
    default: return { label: 'piezas', factor: 1, decimals: false };
  }
}

// entryTotal suma empaques y suelto en la unidad base. null si las dos casillas están vacías:
// vacío es «no contado», y no se confunde con 0 («no hay»).
export function entryTotal(packs: string, packQtyInBase: string | null, loose: string, looseFactor: number): number | null {
  const p = parseNumero(packs);
  const l = parseNumero(loose);
  if (p.estado === 'ausente' && l.estado === 'ausente') return null;
  if (p.estado === 'invalido' || l.estado === 'invalido') return null;
  const pack = packQtyInBase ? Number(packQtyInBase) : 0;
  const total = (p.estado === 'valido' ? p.valor * pack : 0) + (l.estado === 'valido' ? l.valor * looseFactor : 0);
  return Number(total.toFixed(4));
}

// diffWords dice una diferencia de conteo en palabras de anaquel: si se contó más de lo que el
// sistema decía, «sobraban»; si menos, «faltaban». Nunca una existencia negativa a secas.
export function diffWords(delta: number, baseCode: string): string {
  if (delta === 0) return 'cuadra';
  return `${delta > 0 ? 'sobraban' : 'faltaban'} ${formatBaseQty(Math.abs(delta), baseCode)}`;
}

// isBigDiff marca una diferencia que vale la pena revisar antes de cerrar: más de una cuarta parte
// de lo contado (o de lo que había), o cualquiera cuando el sistema estaba en negativo.
export function isBigDiff(delta: number, counted: number, system: number): boolean {
  if (system < 0) return true;
  const scale = Math.max(Math.abs(counted), Math.abs(system), 1);
  return Math.abs(delta) / scale > 0.25;
}

// countedWords dice lo contado como se contó: «3 kg», «12 piezas».
export function countedWords(qty: string | number, baseCode: string): string {
  return formatBaseQty(round2(Number(qty)), baseCode);
}
