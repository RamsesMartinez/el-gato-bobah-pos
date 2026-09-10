// Qué métodos le faltan al operador por contar antes de poder cerrar el turno.
//
// Nace de un corte real: se cerró con el efectivo declarado en $0 y quedó registrado un faltante
// de $1,662 que no era dinero perdido — nadie capturó el conteo y el campo vacío se guardó como
// cero. Un arqueo con un faltante inventado es peor que no tener arqueo: manda a buscar dinero que
// está en el cajón, y desconfía del sistema para siempre.
//
// Un cero ESCRITO sí es válido: puede no haber efectivo. Lo que no vale es el campo en blanco.

import { round2 } from '../../domain/numeros';

export interface MetodoPorContar {
  methodId: number;
  name: string;
  // kind viene del servidor para saber cuál método es el del CAJÓN sin compararlo por nombre: los
  // métodos de plataforma en efectivo también tocan el cajón, así que solo `kind` los distingue.
  kind: string;
  expected: string;
  autoDeclare: boolean;
}

// faltanPorContar devuelve los métodos que exigen conteo físico, esperan dinero y siguen sin
// capturar. Si devuelve algo, el cierre no debe proceder.
export function faltanPorContar(
  totales: MetodoPorContar[],
  capturado: Record<string, string>,
): MetodoPorContar[] {
  return totales.filter((t) => {
    // Los que se autodeclaran los resuelve el servidor: el cajero no captura nada.
    if (t.autoDeclare) return false;
    // Un método que no esperaba nada no obliga a capturar: no hubo movimiento que contar.
    if (Number(t.expected) === 0) return false;
    const v = capturado[String(t.methodId)];
    return v === undefined || v.trim() === '';
  });
}

// KIND_EFECTIVO es el método del cajón: el único que se cuenta por denominaciones.
export const KIND_EFECTIVO = 'efectivo';

export interface DiferenciasDelCierre {
  // Por método, y solo de los que YA tienen cifra: undefined es "todavía no se captura", que no es
  // cero. Tratarlo como cero es el defecto que inventó un faltante de $1,662 en un corte real.
  porMetodo: Record<number, number>;
  total: number;
  // completo dice si el total ya incluye a todos los métodos que exigen captura. Un total que se
  // presenta como definitivo estando incompleto es peor que no mostrarlo.
  completo: boolean;
}

// diferenciasDelCierre calcula, EN EL CLIENTE, lo que el arqueo va a reportar — antes de cerrar.
//
// Existe porque la tabla del cierre en vivo tenía Método / Esperado / Declarado y nada más: la
// única tabla con columna de diferencia se pinta en el diálogo POSTERIOR al cierre, cuando el
// servidor ya cerró el turno y no hay nada que corregir. Es FR-005, y el servidor sigue siendo
// quien calcula la cifra que se guarda.
export function diferenciasDelCierre(
  totales: MetodoPorContar[],
  declarado: Record<number, number | undefined>,
): DiferenciasDelCierre {
  const porMetodo: Record<number, number> = {};
  let total = 0;
  let completo = true;
  for (const t of totales) {
    const esperado = Number(t.expected);
    // Los que se autodeclaran cuadran por construcción: el servidor los declara con su esperado.
    if (t.autoDeclare) {
      porMetodo[t.methodId] = 0;
      continue;
    }
    const d = declarado[t.methodId];
    if (d === undefined) {
      // Un método que no esperaba nada y no se capturó no deja el cierre incompleto: no hubo
      // movimiento que contar. Es el mismo criterio de `faltanPorContar`.
      if (esperado !== 0) completo = false;
      continue;
    }
    const dif = round2(d - esperado);
    porMetodo[t.methodId] = dif;
    total = round2(total + dif);
  }
  return { porMetodo, total, completo };
}
