// Contar el cajón en la pantalla: piezas × valor, en vivo.
//
// El servidor recalcula el total desde las piezas y descarta el que mande el cliente (FR-003), así
// que esta suma NO es la que se guarda. Existe porque el operador tiene que ver el total mientras
// cuenta —es la razón de ser de la feature— y porque el aviso de faltante del cierre se arma con
// ella antes de tocar el botón.
//
// Por eso los casos de `conteo.test.ts` son los mismos que los de `conteo_test.go`: dos sumas del
// mismo dinero que no coinciden dejan al operador firmando un arqueo distinto del que vio.

import { parseNumero, round2 } from '../../domain/numeros';

// Denominacion es una pieza del catálogo, tal como la manda `GET /cash/denominations`. `value` viaja
// como string porque es una columna `numeric(10,2)`.
export interface Denominacion {
  id: number;
  value: string;
  isCoin: boolean;
}

// PiezasTecleadas: lo que hay escrito en el campo de cada denominación, por id. Es TEXTO y no
// número a propósito: el estado de la pantalla es lo que el operador teclea, incluido lo que
// todavía no es un número.
export type PiezasTecleadas = Record<number, string>;

export type CampoDePiezas =
  | { estado: 'vacio' }
  | { estado: 'valido'; piezas: number }
  | { estado: 'invalido'; motivo: 'formato' | 'negativo' | 'fraccion' };

// leerPiezas distingue las tres cosas que el campo puede ser, y esa es toda su razón de existir.
//
// VACÍO NO ES CERO. El campo es el control principal —se cuenta el montón y se escribe— así que
// borrarlo para reescribirlo es un estado por el que se pasa todo el tiempo, y tratarlo como cero
// mandaría un renglón que el operador no capturó.
//
// FRACCIÓN es el motivo que `numeros.ts` no puede dar: ahí se leen dinero y stock, que sí llevan
// decimales. Media moneda no existe, y aceptarla arma un total que el cajón no puede formar.
export function leerPiezas(texto: string): CampoDePiezas {
  const n = parseNumero(texto);
  if (n.estado === 'ausente') return { estado: 'vacio' };
  if (n.estado === 'invalido') return { estado: 'invalido', motivo: n.motivo };
  if (!Number.isInteger(n.valor)) return { estado: 'invalido', motivo: 'fraccion' };
  return { estado: 'valido', piezas: n.valor };
}

export interface RenglonDeConteo {
  denominationId: number;
  pieces: number;
}

export interface Conteo {
  total: number;
  // Solo lo que HAY: una denominación en cero no genera renglón, igual que en el esquema.
  renglones: RenglonDeConteo[];
  // Los ids cuyo campo tiene algo que no es un número de piezas. Quien llama apaga el botón de
  // confirmar mientras esto traiga algo: el total no las incluye, y un total que se ve completo sin
  // estarlo es peor que un error visible.
  invalidas: number[];
}

// ResultadoDelConteo es lo que la hoja entrega cuando el operador confirma: los renglones contados,
// o el total escrito con su motivo. Nunca los dos (FR-014).
//
// No usa los nombres del endpoint —`openingCash`, `declared`— porque la misma hoja sirve para abrir
// y para cerrar, y cada uno manda el efectivo en un campo distinto. Traducirlo es de quien llama.
//
// LOS DOS CAMINOS LLEVAN `total`, y en el de las piezas es informativo: el servidor lo recalcula
// desde los renglones y descarta el que mande el cliente (FR-003). Viaja porque la pantalla del
// cierre tiene que mostrar la diferencia ANTES de confirmar (FR-005), y para eso necesita la cifra
// en pesos sin volver a multiplicar por su cuenta.
export type ResultadoDelConteo =
  | { counts: RenglonDeConteo[]; total: number }
  | { total: number; manualReason: string };

// armarConteo recorre el CATÁLOGO, no lo tecleado.
//
// Es lo que hace que el orden de los renglones sea el del cajón (de mayor a menor) y que una
// captura de una denominación que ya no se ofrece —cambió la moneda, se retiró el billete— no
// viaje: el servidor rechazaría el conteo entero por un id que el operador ya no puede ver.
export function armarConteo(catalogo: Denominacion[], tecleadas: PiezasTecleadas): Conteo {
  const renglones: RenglonDeConteo[] = [];
  const invalidas: number[] = [];
  let total = 0;
  for (const d of catalogo) {
    const campo = leerPiezas(tecleadas[d.id] ?? '');
    if (campo.estado === 'invalido') {
      invalidas.push(d.id);
      continue;
    }
    if (campo.estado === 'vacio' || campo.piezas === 0) continue;
    renglones.push({ denominationId: d.id, pieces: campo.piezas });
    total = round2(total + round2(Number(d.value) * campo.piezas));
  }
  return { total, renglones, invalidas };
}
