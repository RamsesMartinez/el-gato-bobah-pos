import type { TipsDecision, TipsPending } from '../../api/backoffice';

// Reglas del reparto de propinas que la pantalla necesita ANTES de mandar (spec 032). El servidor
// las vuelve a aplicar: aquí solo sirven para que el botón diga lo que va a pasar.

// Reparto parejo en pesos enteros; los centavos y lo que no alcanza un peso por persona siguen
// pendientes (decisión del dueño, 2026-10-09). `null` = no hay reparto posible.
export function repartoParejo(pendiente: number, personas: number): { cada: number; sobrante: number } | null {
  if (personas <= 0) return null;
  const centavos = Math.round(pendiente * 100);
  const cada = Math.floor(centavos / 100 / personas);
  if (cada < 1) return null;
  return { cada, sobrante: (centavos - cada * personas * 100) / 100 };
}

// Montos tecleados en modo ajustado. Un campo vacío no es cero (`Number('')` sí lo es).
export function validarAjustado(pendiente: number, montos: string[]): string | null {
  let total = 0;
  for (const m of montos) {
    const t = m.trim();
    const n = Number(t);
    if (t === '' || !Number.isFinite(n) || n <= 0) return 'Escribe cuánto recibe cada persona';
    if (!Number.isInteger(n)) return 'La propina se entrega en pesos enteros';
    total += n;
  }
  if (total * 100 > Math.round(pendiente * 100)) return 'No hay tanta propina por entregar';
  return null;
}

// Hace falta decidir solo si hay al menos un peso entregable: un sobrante de centavos se hereda
// solo (EB-45), igual que en el servidor.
export function faltaDecidirPropinas(pendiente: TipsPending | null | undefined, decision: TipsDecision | null): boolean {
  return Number(pendiente?.total ?? 0) >= 1 && decision === null;
}
