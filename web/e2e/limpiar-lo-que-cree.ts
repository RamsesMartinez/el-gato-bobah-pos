import { existsSync, readFileSync, unlinkSync } from 'node:fs';
import { API, MARCA, pedidosEnCurso, tokenDeApi } from './ambiente';
import { randomUUID } from 'node:crypto';

// La suite cobra y cierra lo que ella misma abrió.
//
// Corre contra un ambiente compartido con personas: un pedido de prueba que se queda abierto
// aparece en la barra del POS, suma a "por cobrar" y bloquea el cierre de caja. Dejar esa basura no
// es un detalle de higiene — es hacerle creer a quien opera que hay dinero pendiente que cobrar.
//
// SOLO toca lo que no estaba antes de empezar (ver marcar-lo-que-ya-estaba.ts). Sin esa marca no
// hace nada: es preferible dejar basura a cerrarle a alguien una cuenta viva.
export default async function limpiarLoQueCree() {
  if (!existsSync(MARCA)) {
    console.warn('[e2e] sin marca de inicio: no se limpia nada para no tocar pedidos ajenos.');
    return;
  }
  // La marca se lee CON RED. `JSON.parse('')` lanza "Unexpected end of JSON input", y esa excepción
  // tumbaba el teardown ENTERO antes de cerrar un solo pedido — la limpieza moría en silencio
  // justo en el caso en que más hace falta. Pasa de verdad: basta con que una corrida se muera
  // entre que el setup abre el archivo y lo escribe.
  //
  // Sin una marca legible NO se toca nada, que es la misma decisión de siempre: dejar basura es
  // preferible a cerrarle a alguien una cuenta viva.
  let yaEstaban: Set<number>;
  try {
    const crudo = readFileSync(MARCA, 'utf8').trim();
    if (crudo === '') throw new Error('la marca está vacía');
    yaEstaban = new Set<number>(JSON.parse(crudo));
  } catch (e) {
    console.error(`[e2e] la marca de inicio no se pudo leer (${e instanceof Error ? e.message : e}): `
      + 'no se limpia nada. Revisa la barra del POS a mano.');
    unlinkSync(MARCA);
    return;
  }
  unlinkSync(MARCA);

  const jwt = await tokenDeApi();
  const cab = { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' };
  const metodos = await (await fetch(`${API}/payment-methods`, { headers: cab })).json();
  const activos = (metodos.items ?? metodos) as Array<{
    id: number; isActive: boolean; deliveryPlatformId: number | null;
  }>;

  let cerrados = 0;
  const quedaron: string[] = [];
  for (const o of await pedidosEnCurso(jwt)) {
    if (yaEstaban.has(o.id)) continue;
    // Entregar primero: un pedido cobrado pero sin entregar sigue en curso y bloquea el corte.
    if (o.enPreparacion) {
      await fetch(`${API}/orders/${o.id}/deliver`, { method: 'POST', headers: cab, body: '{}' });
    }
    const falta = Number(o.outstanding);
    if (falta <= 0) { cerrados++; continue; }
    // Un pedido de plataforma solo acepta el método DE SU plataforma; con el efectivo del mostrador
    // el servidor lo rechaza con PAYMENT_METHOD_PLATFORM.
    const metodo = activos.find((m) => m.isActive !== false && m.deliveryPlatformId === o.deliveryPlatformId);
    if (!metodo) { quedaron.push(`#${o.number} (sin método para su plataforma)`); continue; }
    const r = await fetch(`${API}/orders/${o.id}/pay`, {
      method: 'POST',
      headers: cab,
      body: JSON.stringify({ methodId: metodo.id, amount: falta, tip: 0, clientUuid: randomUUID() }),
    });
    if (r.ok) cerrados++;
    else quedaron.push(`#${o.number} (${r.status})`);
  }
  console.log(`[e2e] ${cerrados} pedidos de prueba cobrados y cerrados.`);
  // Se GRITA lo que no se pudo cerrar: una limpieza que falla en silencio es peor que no tenerla,
  // porque nadie vuelve a revisar la barra.
  if (quedaron.length) {
    console.error(`[e2e] QUEDARON ABIERTOS: ${quedaron.join(', ')} — hay que cerrarlos a mano.`);
  }
}
