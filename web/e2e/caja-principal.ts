import { API, tokenDeApi } from './ambiente';
import { marcarEntrada, marcarSalida } from './asistencia';

// La suite de caja da por hecho que la caja principal está abierta, como en un local que opera: el
// POS no deja capturar sin turno, y el cierre, la propina y la devolución se prueban sobre él. Un
// respaldo de producción restaurado la trae cerrada (se tomó de noche), y sin esto la mitad de la
// suite se salta o falla en el primer paso.
//
// Solo contra la base local: en un ambiente compartido el turno es de una persona y abrirlo o
// cerrarlo le deja un corte que no hizo. Lo que se abre aquí lo cierra el teardown, y nada más.
const LOCAL = /^https?:\/\/(localhost|127\.0\.0\.1)[:/]/.test(API);
const FONDO = 500;

type Caja = { id: number; isPrimary: boolean; openSessionId: number | null };

async function pedir(jwt: string, metodo: string, ruta: string, cuerpo?: unknown) {
  const r = await fetch(`${API}${ruta}`, {
    method: metodo, headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' },
    body: cuerpo === undefined ? undefined : JSON.stringify(cuerpo),
  });
  if (!r.ok) throw new Error(`${metodo} ${ruta}: ${r.status} ${await r.text()}`);
  return r.json();
}

// abrirLaPrincipalSiEstaCerrada devuelve el id de la caja que abrió, o null si no abrió nada.
export async function abrirLaPrincipalSiEstaCerrada(): Promise<number | null> {
  if (!LOCAL) return null;
  const jwt = await tokenDeApi();
  const cajas = (await pedir(jwt, 'GET', '/cash-registers')) as { items: Caja[] };
  const principal = cajas.items.find((c) => c.isPrimary);
  if (!principal || principal.openSessionId !== null) return null;
  // Con la asistencia encendida, abrir pide la entrada de quien abre (spec 046).
  await marcarEntrada(jwt);
  // Con datos reales, lo contado casi nunca coincide con el último cierre y la apertura pide motivo
  // (OPENING_REASON_REQUIRED). Sin él la caja no se abría, en silencio, y toda prueba que entra por
  // Vender esperaba 30 s a «No hay caja abierta» (#193). El motivo solo se valida cuando hace falta.
  await pedir(jwt, 'POST', '/cash-sessions', {
    registerId: principal.id, openingCash: FONDO, manualReason: 'apertura de la suite e2e',
    openingReason: 'other', openingReasonNote: 'apertura de la suite e2e',
  });
  return principal.id;
}

// cerrarLaPrincipal cierra el turno que abrió la suite. Se cuenta el fondo y cada terminal que cobró
// en cero: con el arqueo ciego lo esperado no viaja, y la diferencia que quede es de un turno de
// prueba en la base local.
export async function cerrarLaPrincipal(registerId: number) {
  const jwt = await tokenDeApi();
  // Cerrar pide que quien abrió ya haya marcado su salida (spec 046).
  await marcarSalida(jwt);
  const turno = (await pedir(jwt, 'GET', `/cash-sessions/current?registerId=${registerId}`)) as
    { terminalsToCount?: Array<{ terminalId: number }> };
  const terminalCounts = Object.fromEntries((turno.terminalsToCount ?? []).map((t) => [String(t.terminalId), 0]));
  await pedir(jwt, 'POST', '/cash-sessions/close', {
    registerId, countedCash: FONDO, manualReason: 'cierre de la suite e2e', tipsDecision: 'quedan_en_caja', terminalCounts,
  });
}
