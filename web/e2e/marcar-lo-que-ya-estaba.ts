import { writeFileSync } from 'node:fs';
import { MARCA, cuentasVivas, tokenDeApi } from './ambiente';

// Antes de correr nada, anota qué pedidos y qué cuentas en captura YA estaban abiertos.
//
// Es lo que le permite a la limpieza distinguir su propia basura de la de una persona: la suite
// crea cuentas contra un ambiente compartido, y cerrar a ciegas todo lo pendiente le cerraría al
// dueño una cuenta que dejó abierta a propósito.
export default async function marcarLoQueYaEstaba() {
  try {
    const jwt = await tokenDeApi();
    const vivas = await cuentasVivas(jwt);
    const pedidos = vivas.filter((c) => c.kind === 'order').map((c) => c.orderId);
    const cuentas = vivas.filter((c) => c.kind === 'draft').map((c) => c.draftId);
    writeFileSync(MARCA, JSON.stringify({ pedidos, cuentas }));
    console.log(`[e2e] ${pedidos.length} pedidos y ${cuentas.length} cuentas ya estaban abiertos; no se tocan al terminar.`);
  } catch (e) {
    // Sin marca la limpieza no corre, y eso es lo correcto: es preferible dejar basura a cerrar
    // cuentas de alguien más.
    console.warn('[e2e] no se pudo anotar lo que ya estaba abierto:', e);
  }
}
