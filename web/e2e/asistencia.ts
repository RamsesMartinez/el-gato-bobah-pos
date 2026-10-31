import { expect, type Page } from '@playwright/test';
import { API, tokenDeApi } from './ambiente';

// Marcar entrada y salida de quien corre la suite, por el mismo `POST /attendance/clock` que usa la
// pantalla (spec 046). Con la asistencia encendida, abrir caja pide la entrada de quien abre y
// cerrarla pide la salida de quien la abrió: una prueba de caja que no marque se queda en el aviso.
//
// El PIN viene de `E2E_PIN` y no se escribe en ningún archivo. Sin él, la prueba falla diciendo
// qué falta, en vez de un 409 de la caja que se leería como un defecto.
const PIN = process.env.E2E_PIN;

type Roster = { enabled: boolean; people: Array<{ userId: number; openSince?: string }> };

async function pedir<T>(jwt: string, ruta: string, init: RequestInit = {}): Promise<T> {
  const r = await fetch(`${API}${ruta}`, {
    ...init, headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json', ...(init.headers ?? {}) },
  });
  if (!r.ok) throw new Error(`${init.method ?? 'GET'} ${ruta}: ${r.status} ${await r.text()}`);
  return (await r.json()) as T;
}

// estado: si la asistencia aplica a quien corre la suite y si está dentro. `null` = no aplica.
async function estado(jwt: string): Promise<{ yo: number; dentro: boolean } | null> {
  const me = await pedir<{ id: number }>(jwt, '/auth/me');
  const roster = await pedir<Roster>(jwt, '/attendance/roster');
  if (!roster.enabled) return null;
  const yo = roster.people.find((p) => p.userId === me.id);
  if (!yo) return null; // no lleva asistencia: la caja no le pide nada
  return { yo: me.id, dentro: !!yo.openSince };
}

async function checar(jwt: string, userId: number, expected: 'in' | 'out') {
  if (!PIN) {
    throw new Error('La empresa tiene la asistencia encendida: para abrir o cerrar caja la suite marca '
      + 'entrada y salida, y para eso necesita E2E_PIN (el PIN del usuario de E2E_USER).');
  }
  await pedir(jwt, '/attendance/clock', { method: 'POST', body: JSON.stringify({ userId, pin: PIN, expected }) });
}

// marcarEntrada deja a quien corre la suite con su entrada marcada, si la asistencia le aplica.
export async function marcarEntrada(jwt?: string) {
  const t = jwt ?? await tokenDeApi();
  const e = await estado(t);
  if (e && !e.dentro) await checar(t, e.yo, 'in');
}

// marcarSalida deja a quien corre la suite fuera, para que el cierre de la caja que abrió no se
// detenga en el aviso.
export async function marcarSalida(jwt?: string) {
  const t = jwt ?? await tokenDeApi();
  const e = await estado(t);
  if (e && e.dentro) await checar(t, e.yo, 'out');
}

// marcarSalidaEnElCierre marca, desde el aviso del cierre, la salida de quien abrió la caja: la
// misma hoja de PIN que usa el cajero. Sin aviso no hace nada.
export async function marcarSalidaEnElCierre(page: Page) {
  const boton = page.getByRole('button', { name: /^Marcar salida de / });
  // El aviso o el cierre de siempre: hasta que la consulta responde no se pinta ninguno.
  const cierre = page.getByText('Cierre — declarado por método');
  await expect(boton.or(cierre).first()).toBeVisible({ timeout: 30_000 });
  if (!(await boton.isVisible())) return;
  if (!PIN) throw new Error('El cierre pide la salida de quien abrió y falta E2E_PIN para marcarla.');
  await boton.click();
  const hoja = page.getByRole('dialog').last();
  for (const d of PIN) await hoja.getByRole('button', { name: d, exact: true }).click();
  await hoja.getByRole('button', { name: 'Salida' }).click();
  await expect(page.getByText(/^Salida de .* marcada/)).toBeVisible({ timeout: 30_000 });
}
