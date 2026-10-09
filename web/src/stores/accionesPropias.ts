
// LO QUE HIZO ESTA TABLETA (spec 030, FR-017).
//
// El aviso «X se cobró en otra tableta» tiene que callar ante lo propio. Separarlo por tiempo —«lo
// que llegue en los 3 s siguientes es eco»— fallaba en las tres acciones que se probaron: el eco
// de un cobro llega cuando la hoja ya se cerró, y el de quitar un producto, después de la hoja del
// motivo. Lo propio se reconoce por lo que DEJÓ:
//
//   - mientras una acción de esta tableta está en vuelo, todo cambio de la cuenta abierta es suyo;
//   - al terminar, la cuenta abierta se relee y esa foto queda anotada como «eco». Cuando el
//     servidor la vuelva a mandar —por el canal de eventos, por el sondeo— no es un cambio de nadie.
//
// Una foto distinta a las anotadas es de otra tableta, llegue cuando llegue.

// Lo que se compara de una cuenta para decidir si cambió: vive aquí y no en la pantalla porque la
// hoja de cobro (shared) también escribe sobre la cuenta abierta.
export interface FotoDeCuenta {
  nombre: string;
  // De la cuenta en captura (capturando · enviada · descartada) o del pedido.
  estado: string;
  falta: number;
  // Cantidad por renglón.
  renglones: Record<string, number>;
}

const MAX_ECOS = 12;
let enCurso = 0;
let ecos: string[] = [];
let releer: (() => Promise<FotoDeCuenta | undefined>) | null = null;

export function firma(f: FotoDeCuenta): string {
  const renglones = Object.entries(f.renglones).sort(([a], [b]) => a.localeCompare(b));
  return JSON.stringify([f.nombre, f.estado, f.falta.toFixed(2), renglones]);
}

// anotarEco guarda una foto que produjo esta tableta: cuando el servidor la repita, no es ajena.
export function anotarEco(f: FotoDeCuenta | undefined): void {
  if (!f) return;
  const k = firma(f);
  ecos = [k, ...ecos.filter((e) => e !== k)].slice(0, MAX_ECOS);
}

export function esPropia(f: FotoDeCuenta): boolean {
  return enCurso > 0 || ecos.includes(firma(f));
}

// alReleer registra cómo leer la cuenta abierta. Lo hace `useCuenta`, que es quien sabe cuál es.
export function alReleer(fn: () => Promise<FotoDeCuenta | undefined>): () => void {
  releer = fn;
  return () => { if (releer === fn) releer = null; };
}

// accionPropia envuelve una escritura de esta tableta sobre una cuenta: cobrar, quitar, cancelar,
// descartar, mandar. Se usa en TODAS: la que se olvide vuelve a avisar como si fuera de otra.
export async function accionPropia<T>(fn: () => Promise<T>): Promise<T> {
  enCurso += 1;
  try {
    return await fn();
  } finally {
    try {
      anotarEco(await releer?.());
    } catch {
      // Sin la relectura no hay eco anotado; lo peor es un aviso de más, nunca uno de menos.
    } finally {
      enCurso -= 1;
    }
  }
}

export function reiniciarPropias(): void {
  enCurso = 0;
  ecos = [];
}
