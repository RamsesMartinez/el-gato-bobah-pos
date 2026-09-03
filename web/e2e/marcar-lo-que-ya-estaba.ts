import { writeFileSync } from 'node:fs';
import { MARCA, pedidosEnCurso, tokenDeApi } from './ambiente';

// Antes de correr nada, anota qué pedidos YA estaban abiertos.
//
// Es lo que le permite a la limpieza distinguir su propia basura de la de una persona: la suite
// crea pedidos contra un ambiente compartido, y cobrar a ciegas todo lo pendiente le cerraría al
// dueño una cuenta que dejó abierta a propósito.
export default async function marcarLoQueYaEstaba() {
  try {
    const jwt = await tokenDeApi();
    const ids = (await pedidosEnCurso(jwt)).map((o) => o.id);
    writeFileSync(MARCA, JSON.stringify(ids));
    console.log(`[e2e] ${ids.length} pedidos ya estaban abiertos; no se tocan al terminar.`);
  } catch (e) {
    // Sin marca la limpieza no corre, y eso es lo correcto: es preferible dejar basura a cobrar
    // pedidos de alguien más.
    console.warn('[e2e] no se pudo anotar lo que ya estaba abierto:', e);
  }
}
