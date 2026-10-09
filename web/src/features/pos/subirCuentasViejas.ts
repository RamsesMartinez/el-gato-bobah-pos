import { posApi } from '../../api/pos';
import { toaster } from '../../components/ui/toaster';
import { parseMonto, parseNumero } from '../../domain/numeros';
import { usePosStore } from '../../stores/pos';
import type { DraftHeader, ImportAccount, ImportResult, ServiceType } from '../../types/pos';
import { uuidv5 } from '../../utils/uuidv5';

// SUBIR LAS CUENTAS QUE LA VERSIÓN ANTERIOR GUARDABA EN LA TABLETA (D-12, research R-10).
//
// Hasta la 030 las cuentas vivían en `egb:ticket:v2`. Un deploy no puede tirar lo que alguien
// estaba capturando, así que la primera carga de la versión nueva las sube al servidor y, solo
// cuando TODAS regresan con resultado, borra la llave. Si la red falla, la conserva y lo vuelve a
// intentar en la siguiente carga con las MISMAS llaves: el id de la pestaña es el id de la cuenta y
// cada renglón lleva `uuidv5(lineId, tab.id)`, así que reintentar no duplica nada.
//
// Una pestaña que ya no se entiende —la forma vieja que dejaba la pantalla en blanco, caso 18— se
// salta con aviso en vez de bloquear a las demás.

const LLAVE = 'egb:ticket:v2';
const TANDA = 20;
let yaCorrio = false;

export function reiniciarSubida(): void {
  yaCorrio = false;
}

interface PestañaVieja {
  id: string;
  folioName?: string;
  lines: Array<{
    lineId: string; productId: number; qty: number; notes?: string;
    modifiers?: Array<{ optionId: number; qty: number }>;
  }>;
  envio?: string;
  descuento?: string;
  descuentoModo?: 'monto' | 'pct';
  serviceType?: ServiceType;
  customerName?: string;
  platformId?: number | null;
  platformOrderRef?: string;
}

function esPestaña(t: unknown): t is PestañaVieja {
  if (typeof t !== 'object' || t === null) return false;
  const p = t as Record<string, unknown>;
  if (typeof p.id !== 'string' || !Array.isArray(p.lines)) return false;
  return p.lines.every((l) => {
    if (typeof l !== 'object' || l === null) return false;
    const r = l as Record<string, unknown>;
    return typeof r.lineId === 'string' && typeof r.productId === 'number' && typeof r.qty === 'number'
      && (r.modifiers === undefined || Array.isArray(r.modifiers));
  });
}

function cabecera(t: PestañaVieja): DraftHeader {
  const h: DraftHeader = {
    serviceType: t.serviceType === 'domicilio' ? 'domicilio' : 'mostrador',
    customerName: t.customerName?.trim() || null,
    platformId: typeof t.platformId === 'number' ? t.platformId : null,
    platformOrderRef: t.platformOrderRef?.trim() || null,
  };
  const envio = parseMonto(t.envio ?? '');
  if (envio.estado === 'valido') h.deliveryFee = envio.valor.toFixed(2);
  const modo = t.descuentoModo === 'pct' ? 'pct' : 'monto';
  const d = modo === 'pct' ? parseNumero(t.descuento ?? '') : parseMonto(t.descuento ?? '');
  if (d.estado === 'valido' && d.valor > 0) {
    h.discount = modo === 'pct' ? { percent: String(d.valor) } : { amount: d.valor.toFixed(2) };
  }
  return h;
}

function aCuenta(t: PestañaVieja): ImportAccount {
  return {
    id: t.id,
    folioName: t.folioName ?? '',
    header: cabecera(t),
    lines: t.lines.map((l) => ({
      opId: uuidv5(l.lineId, t.id),
      productId: l.productId,
      qty: String(l.qty),
      modifiers: (l.modifiers ?? []).map((m) => ({ optionId: m.optionId, qty: m.qty })),
      notes: l.notes ?? '',
    })),
  };
}

export async function subirCuentasViejas(): Promise<void> {
  if (yaCorrio) return;
  yaCorrio = true;

  let texto: string | null;
  try {
    texto = localStorage.getItem(LLAVE);
  } catch {
    return;
  }
  if (texto === null) return;
  let crudo: { state?: { tabs?: unknown; activeId?: unknown } } | null;
  try {
    crudo = JSON.parse(texto);
  } catch {
    crudo = null;
  }
  const tabs: unknown[] = Array.isArray(crudo?.state?.tabs) ? crudo.state.tabs : [];
  const activa = crudo?.state?.activeId;

  const validas = tabs.filter(esPestaña);
  if (validas.length < tabs.length) {
    toaster.create({
      title: 'Una cuenta guardada en esta tableta no se pudo recuperar',
      description: 'Las demás ya están en la fila de cuentas.',
      type: 'warning',
    });
  }
  const cuentas = validas.filter((t) => t.lines.length > 0).map(aCuenta);

  const resultados: ImportResult[] = [];
  try {
    for (let i = 0; i < cuentas.length; i += TANDA) {
      const r = await posApi.importDrafts(cuentas.slice(i, i + TANDA));
      resultados.push(...(r.results ?? []));
    }
  } catch {
    // Sin red: la llave se queda y la siguiente carga lo vuelve a intentar.
    return;
  }

  const resueltas = new Set(resultados.map((r) => r.id));
  if (!cuentas.every((c) => resueltas.has(c.id))) return;
  try { localStorage.removeItem(LLAVE); } catch { /* sin almacenamiento: no hay nada que borrar */ }

  const deLaActiva = resultados.find((r) => r.id === activa);
  if (deLaActiva && usePosStore.getState().selected === null) {
    if (deLaActiva.draftId) usePosStore.getState().seleccionar({ kind: 'draft', id: deLaActiva.draftId });
    else if (deLaActiva.orderId) usePosStore.getState().seleccionar({ kind: 'order', id: deLaActiva.orderId });
  }
}
