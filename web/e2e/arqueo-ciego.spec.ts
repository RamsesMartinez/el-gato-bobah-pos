import { test, expect, type Page } from '@playwright/test';
import { API, USUARIO, EMPRESA, PASSWORD, tokenDeApi } from './ambiente';

// ARQUEO CIEGO EN LA TABLETA (spec 015, US3).
//
// Quien cuenta el cajón no ve lo que el sistema espera. Lo que los tests de integración ya prueban
// es que el servidor no manda la cifra; lo que solo se ve aquí es que la pantalla tampoco la pinta
// y que, sin el esperado, el cierre SIGUE sabiendo que falta contar.
//
// Ese último es el borde que importa: si la pantalla dedujera "falta capturar" de comparar contra
// el esperado, con el esperado en null concluiría que no falta nada y el botón rojo quedaría
// habilitado sobre una pantalla en blanco — un cierre firmado sin contar el cajón.
//
// EL INTERRUPTOR SE DEVUELVE COMO ESTABA. El ambiente lo comparte una persona y dejarlo encendido
// le esconde las cifras del corte sin que nadie se lo haya pedido.

async function entrar(page: Page) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  const usuario = page.getByPlaceholder('usuario@empresa');
  if (await usuario.isVisible().catch(() => false)) {
    await usuario.fill(`${USUARIO}@${EMPRESA}`);
    await page.getByPlaceholder('Contraseña').fill(PASSWORD);
    await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  }
  await expect(page.getByRole('button', { name: 'Salir' })).toBeVisible({ timeout: 30_000 });
}

// ponerElCiego lo mueve por la API y no por la pantalla: el interruptor se prueba en su propio
// caso, y aquí lo que se está probando es el cierre. Devuelve cómo estaba para restituirlo.
async function ponerElCiego(encendido: boolean): Promise<boolean> {
  const jwt = await tokenDeApi();
  const antes = await fetch(`${API}/business-settings`, { headers: { Authorization: `Bearer ${jwt}` } });
  if (!antes.ok) throw new Error(`GET /business-settings: ${antes.status}`);
  const previo = (await antes.json()).blindCashCount === true;
  const r = await fetch(`${API}/business-settings`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${jwt}` },
    body: JSON.stringify({ blindCashCount: encendido }),
  });
  if (!r.ok) throw new Error(`PUT /business-settings: ${r.status}`);
  return previo;
}

// cifrasEsperadas junta todas las claves `expected` de la respuesta, a cualquier profundidad. Una
// cifra escondida dos niveles adentro es igual de legible con las herramientas del navegador.
function cifrasEsperadas(valor: unknown, encontradas: unknown[] = []): unknown[] {
  if (Array.isArray(valor)) { for (const v of valor) cifrasEsperadas(v, encontradas); return encontradas; }
  if (valor && typeof valor === 'object') {
    for (const [k, v] of Object.entries(valor)) {
      if (k === 'expected' && v !== null) encontradas.push(v);
      cifrasEsperadas(v, encontradas);
    }
  }
  return encontradas;
}

// reconstruible: la suma con la que se llega al esperado SIN leer ninguna clave llamada `expected`.
//
// Buscar esa palabra no alcanzaba, y creer que sí es lo que dejó pasar el hallazgo: la fuga viajaba
// en claves `amount` y `total`, dentro del desglose por método que la pantalla pinta arriba de la
// tabla del cierre. Fondo + neto + los ingresos de los métodos del cajón = el esperado exacto.
type TurnoAbierto = {
  openingCash: string; netMovements: string;
  drawer: { methodIds: number[] } | null;
  totals: Array<{ methodId: number; name: string }>;
  breakdown?: { ingresos?: Array<{ method: string; total: string }> };
  cashiers?: Array<{ cash: string }>;
};
function reconstruible(s: TurnoAbierto): { porDesglose: number; porCajero: number } {
  const base = Number(s.openingCash) + Number(s.netMovements);
  const delCajon = new Set((s.drawer?.methodIds ?? [])
    .map((id) => s.totals.find((t) => t.methodId === id)?.name)
    .filter(Boolean) as string[]);
  const porDesglose = (s.breakdown?.ingresos ?? [])
    .filter((i) => delCajon.has(i.method))
    .reduce((suma, i) => suma + Number(i.total), base);
  const porCajero = (s.cashiers ?? []).reduce((suma, c) => suma + Number(c.cash), base);
  return { porDesglose, porCajero };
}

// esperadoDelCajon: cuánto espera el cajón de la caja que está vendiendo, leído CON EL CIEGO
// APAGADO. Un cajón que no espera nada no obliga a contar —es el mismo criterio con el que un
// método sin movimiento no pide cifra—, así que sin esto el caso del botón bloqueado se prueba
// contra un turno donde no hay nada que bloquear y pasa o falla por el ambiente.
async function esperadoDelCajon(): Promise<number | null> {
  const jwt = await tokenDeApi();
  const cajas = await fetch(`${API}/cash-registers`, { headers: { Authorization: `Bearer ${jwt}` } });
  if (!cajas.ok) throw new Error(`GET /cash-registers: ${cajas.status}`);
  for (const c of (await cajas.json()).items ?? []) {
    const r = await fetch(`${API}/cash-sessions/current?registerId=${c.id}`,
      { headers: { Authorization: `Bearer ${jwt}` } });
    if (!r.ok) continue;
    const sesion = await r.json();
    if (sesion?.drawer?.expected != null) return Number(sesion.drawer.expected);
  }
  return null;
}

let comoEstaba = false;

test.beforeEach(async () => { comoEstaba = await ponerElCiego(false); });
test.afterEach(async () => { await ponerElCiego(comoEstaba); });

test('B1 · con el arqueo ciego la pantalla del cierre no tiene columna de esperado, y la respuesta tampoco trae la cifra', async ({ page }) => {
  // Se lee ANTES de encender el ciego: es la cifra contra la que se comprueba que nada la
  // reconstruya después. Con el ciego ya puesto no habría contra qué comparar.
  const esperado = await esperadoDelCajon();
  await ponerElCiego(true);
  const respuestas: unknown[] = [];
  page.on('response', async (r) => {
    if (!/\/cash-sessions/.test(r.url()) || !r.ok()) return;
    try { respuestas.push(await r.json()); } catch { /* no era JSON */ }
  });

  await entrar(page);
  await page.goto('/caja');
  await expect(page.getByRole('tab', { name: 'Cajas' })).toBeVisible({ timeout: 30_000 });
  const tabla = page.getByText('Cierre — declarado por método');
  test.skip(!(await tabla.isVisible().catch(() => false)), 'no hay una caja abierta en el ambiente: no se abre la de alguien más');

  // NI LA COLUMNA. Con todos los esperados en null la columna entera sobra, y una de rayas invita
  // a preguntarse qué se rompió.
  await expect(page.getByRole('columnheader', { name: 'Esperado' })).toHaveCount(0);
  // Ni la cifra del cajón, que es la que el conteo va a contradecir.
  await expect(page.getByRole('row', { name: /^Cajón/ })).toBeVisible();

  // Y NI EN LA RESPUESTA. Ocultarlo en el cliente deja la cifra a un F12 de distancia: el control
  // dejaría de serlo.
  expect(respuestas.length, 'no se observó ninguna respuesta de /cash-sessions').toBeGreaterThan(0);
  const filtradas = respuestas.flatMap((r) => cifrasEsperadas(r));
  expect(filtradas, `el servidor mandó ${filtradas.length} cifras esperadas con el arqueo ciego encendido`).toEqual([]);

  // Y LO DERIVADO TAMPOCO: ninguna de las dos sumas puede alcanzar la cifra oculta.
  if (esperado !== null && esperado > 0) {
    const abierto = respuestas.find((r) => (r as TurnoAbierto).drawer !== undefined) as TurnoAbierto | undefined;
    expect(abierto, 'no se observó la vista del turno abierto').toBeTruthy();
    const { porDesglose, porCajero } = reconstruible(abierto!);
    expect(Math.abs(porDesglose - esperado),
      `el desglose reconstruye el esperado (${porDesglose}): quien cuenta lo lee en pantalla sin abrir nada`)
      .toBeGreaterThan(0.005);
    expect(Math.abs(porCajero - esperado),
      `lo cobrado por cajero reconstruye el esperado (${porCajero})`)
      .toBeGreaterThan(0.005);
  }

  // En pantalla, el resumen del corte tampoco se pinta.
  await expect(page.getByText('Aparece al confirmar el cierre, junto con la diferencia.')).toBeVisible();
});

test('B2 · y el botón de cerrar sigue bloqueado mientras no se cuente el cajón', async ({ page }) => {
  const esperado = await esperadoDelCajon();
  test.skip(esperado === null, 'no hay una caja abierta en el ambiente: no se abre la de alguien más');
  test.skip(esperado === 0, 'el cajón de este turno no espera dinero: no hay nada que obligue a contar');
  await ponerElCiego(true);
  await entrar(page);
  await page.goto('/caja');
  await expect(page.getByRole('tab', { name: 'Cajas' })).toBeVisible({ timeout: 30_000 });
  const cerrar = page.getByRole('button', { name: 'Cerrar caja' });
  test.skip(!(await cerrar.isVisible().catch(() => false)), 'no hay una caja abierta en el ambiente: no se abre la de alguien más');

  // El botón del cajón dice «Contar efectivo» mientras no haya conteo: si dijera un monto, ya se
  // contó y este caso no aplica.
  const contar = page.getByRole('button', { name: 'Contar efectivo' });
  test.skip(!(await contar.isVisible().catch(() => false)), 'el cajón de este turno ya está contado');

  await expect(cerrar, 'el cierre quedó habilitado sin haber contado el cajón: con el esperado en null, la pantalla creyó que no faltaba nada').toBeDisabled();
});
