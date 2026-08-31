import type { MenuGroup } from '../../types/pos';

// Recordar cómo se pidió un producto la última vez, en ESTA tableta.
//
// El pre-marcado por historial ya deja la hoja en dos taps, pero solo cuando hay señal: en
// multi-selección exige opciones rankeadas, y sin ventas capturadas no hay ranking. Eso es un
// círculo — no se captura porque cuesta taps, y cuesta taps porque no se ha capturado.
//
// La memoria local lo rompe: desde la segunda venta del producto, la hoja abre con las salsas de la
// vez pasada. La hoja SIGUE ABRIENDO y las marcas se ven, así que el operador puede corregirlas de
// un toque — nunca sale comida que nadie pidió sin que nadie la haya visto.

// groupId → optionId → cuántas veces.
export type Seleccion = Record<number, Record<number, number>>;

// completarConLaUltima rellena solo los grupos OBLIGATORIOS que quedaron sin cumplir.
//
// Los opcionales no se tocan: rellenarlos agregaría un extra que nadie pidió y que se cobra. Y lo
// que el pre-marcado por historial ya eligió gana, porque esa señal sale de lo que de verdad se
// vendió, no de la memoria de una tableta.
export function completarConLaUltima(
  base: Seleccion,
  guardada: Seleccion | null,
  grupos: MenuGroup[],
): Seleccion {
  if (!guardada) return base;
  const out: Seleccion = { ...base };

  for (const g of grupos) {
    if (g.min <= 0) continue;
    const yaElegidas = Object.values(out[g.id] ?? {}).reduce((a, b) => a + b, 0);
    if (yaElegidas >= g.min) continue;

    const recordada = guardada[g.id];
    if (!recordada) continue;

    // Una opción archivada desde la última vez ya no está en el menú: rellenar con ella mandaría a
    // cocina algo que el negocio quitó.
    const existentes = new Set(g.options.map((o) => o.id));
    const del: Record<number, number> = {};
    let total = 0;
    for (const [id, veces] of Object.entries(recordada)) {
      const oid = Number(id);
      if (!existentes.has(oid)) continue;
      const cabe = Math.min(veces, g.max - total);
      if (cabe <= 0) continue;
      del[oid] = cabe;
      total += cabe;
    }
    if (total > 0) out[g.id] = del;
  }
  return out;
}

const LLAVE = 'pos.ultimaCombinacion';

// Lo guardado vive en el navegador de esta tableta: es una comodidad de captura, no un dato del
// negocio. Si se pierde, la hoja vuelve a abrir como hoy y no se rompe nada.
export function guardarCombinacion(productId: number, sel: Seleccion): void {
  try {
    const todo = leerTodo();
    todo[productId] = sel;
    localStorage.setItem(LLAVE, JSON.stringify(todo));
  } catch {
    // Sin almacenamiento (ventana privada, cuota llena) la captura sigue funcionando igual.
  }
}

export function combinacionGuardada(productId: number): Seleccion | null {
  return leerTodo()[productId] ?? null;
}

function leerTodo(): Record<number, Seleccion> {
  try {
    return JSON.parse(localStorage.getItem(LLAVE) || '{}') as Record<number, Seleccion>;
  } catch {
    return {};
  }
}
