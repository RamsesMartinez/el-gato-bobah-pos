// Qué métodos le faltan al operador por contar antes de poder cerrar el turno.
//
// Nace de un corte real: se cerró con el efectivo declarado en $0 y quedó registrado un faltante
// de $1,662 que no era dinero perdido — nadie capturó el conteo y el campo vacío se guardó como
// cero. Un arqueo con un faltante inventado es peor que no tener arqueo: manda a buscar dinero que
// está en el cajón, y desconfía del sistema para siempre.
//
// Un cero ESCRITO sí es válido: puede no haber efectivo. Lo que no vale es el campo en blanco.

import { round2 } from '../../domain/numeros';
import type { ResultadoDelConteo } from './conteo';

export interface MetodoPorContar {
  methodId: number;
  name: string;
  // kind viene del servidor para saber cuál método es el del CAJÓN sin compararlo por nombre: los
  // métodos de plataforma en efectivo también tocan el cajón, así que solo `kind` los distingue.
  kind: string;
  // expected viaja en NULL con el arqueo ciego: lo que la pantalla no debe mostrar no se le manda.
  expected: string | null;
  autoDeclare: boolean;
  // requiresEntry: si ESTE método exige una cifra capturada. Lo decide el servidor.
  requiresEntry: boolean;
}

// ArqueoDelCajon: una cifra esperada, un conteo y una diferencia, para el cajón completo.
export interface ArqueoDelCajon {
  expected: string | null;
  counted: string | null;
  difference: string | null;
  methodIds: number[];
  requiresCount: boolean;
}

// faltanPorContar devuelve los métodos que exigen cifra y siguen sin capturar. Si devuelve algo, el
// cierre no debe proceder.
//
// QUIÉN LO EXIGE ES EL SERVIDOR, y ese cambio no es cosmético. Antes esta función lo deducía de si
// el esperado era cero; con el arqueo ciego el esperado viaja en null, `Number(null)` es 0 y la
// regla vieja concluía que NINGÚN método esperaba dinero: el botón de cerrar quedaba habilitado con
// la pantalla en blanco. Es el faltante inventado de $1,662 por la puerta de atrás, y por eso la
// pregunta se la hace al único que sigue viendo las cifras.
export function faltanPorContar(
  totales: MetodoPorContar[],
  capturado: Record<string, string>,
): MetodoPorContar[] {
  return totales.filter((t) => {
    if (!t.requiresEntry) return false;
    const v = capturado[String(t.methodId)];
    return v === undefined || v.trim() === '';
  });
}

// faltaContarElCajon: si el cierre todavía espera que alguien cuente el cajón.
//
// Por lo mismo que `requiresEntry`, la condición la manda el servidor en `requiresCount` y no se
// deduce de que el esperado sea cero: con el arqueo ciego ese esperado no existe en la pantalla.
export function faltaContarElCajon(
  cajon: ArqueoDelCajon | null | undefined,
  conteo: ResultadoDelConteo | null,
): boolean {
  if (!cajon) return false;
  return cajon.requiresCount && conteo === null;
}

// diferenciaDelCajon: lo que el arqueo va a reportar, antes de firmarlo.
//
// `undefined` —y no cero— cuando no se puede saber: sin conteo todavía, o con el arqueo ciego
// encendido, donde el esperado no viaja. Un cero se leería como "cuadra", que es afirmar algo que
// nadie comprobó.
export function diferenciaDelCajon(
  cajon: ArqueoDelCajon | null | undefined,
  conteo: ResultadoDelConteo | null,
): number | undefined {
  if (!cajon || conteo === null || cajon.expected === null) return undefined;
  return round2(conteo.total - Number(cajon.expected));
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
    // Un método cuyo dinero está en el cajón NO tiene diferencia propia: la del cajón es una sola y
    // se calcula aparte. Repartirla entre métodos es lo que hacía que un faltante y un sobrante se
    // cancelaran y el corte pareciera cuadrado.
    if (!t.requiresEntry) continue;
    const d = declarado[t.methodId];
    if (d === undefined) {
      // Incompleto solo si ESE método exigía captura. El criterio es el mismo de `faltanPorContar`
      // y sale del mismo lugar: el servidor, no una deducción sobre el esperado.
      if (t.requiresEntry) completo = false;
      continue;
    }
    const dif = round2(d - esperado);
    porMetodo[t.methodId] = dif;
    total = round2(total + dif);
  }
  return { porMetodo, total, completo };
}
