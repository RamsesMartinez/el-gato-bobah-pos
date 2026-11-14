import { expect, type Page } from '@playwright/test';
import { API } from './ambiente';
import { iniciarSesion } from './sesion';
import { marcarEntrada, marcarSalida } from './asistencia';

// Turnos de caja armados por la API para las e2e de Caja (#290): abrir, cerrar con un conteo dado y
// entrar a /caja. // Solo contra la base local: abre y cierra turnos, y en un ambiente compartido serían de alguien.
export const LOCAL = /^https?:\/\/(localhost|127\.0\.0\.1)[:/]/.test(API);

export async function pedir(jwt: string, metodo: string, ruta: string, cuerpo?: unknown) {
  const r = await fetch(`${API}${ruta}`, {
    method: metodo, headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' },
    body: cuerpo === undefined ? undefined : JSON.stringify(cuerpo),
  });
  if (!r.ok) throw new Error(`${metodo} ${ruta}: ${r.status} ${await r.text()}`);
  return r.json();
}

export type Caja = { id: number; name: string; isPrimary: boolean; openSessionId: number | null; countsCash?: boolean };

export async function cerrar(jwt: string, registerId: number, contado: number) {
  await marcarSalida(jwt);
  const turno = (await pedir(jwt, 'GET', `/cash-sessions/current?registerId=${registerId}`)) as
    { terminalsToCount?: Array<{ terminalId: number }> };
  const terminalCounts = Object.fromEntries((turno.terminalsToCount ?? []).map((t) => [String(t.terminalId), 0]));
  return pedir(jwt, 'POST', '/cash-sessions/close', {
    registerId, countedCash: contado, manualReason: 'cierre de la suite e2e', tipsDecision: 'quedan_en_caja', terminalCounts,
  }) as Promise<{ id: number }>;
}

export async function abrir(jwt: string, registerId: number, fondo: number, nota: string) {
  await marcarEntrada(jwt);
  return pedir(jwt, 'POST', '/cash-sessions', {
    registerId, openingCash: fondo, manualReason: 'apertura de la suite e2e',
    openingReason: 'last_count_wrong', openingReasonNote: nota,
  }) as Promise<{ id: number }>;
}

export async function entrarACaja(page: Page) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  await expect(page.getByRole('button', { name: 'Cuenta nueva', exact: true })).toBeVisible({ timeout: 30_000 });
  await page.goto('/caja');
  await page.waitForLoadState('networkidle');
}
