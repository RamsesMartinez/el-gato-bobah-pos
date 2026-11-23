// Cuánta prisa hay con un pedido de plataforma.
//
// NO SE PINTAN SEGUNDOS, y no es una decisión estética. Entre el segundo 47 y el 46 no hay ninguna
// decisión distinta que tomar: lo que cambia el comportamiento son dos momentos concretos, y esos
// sí hay que verlos.
//
//   - A los 90 segundos sin respuesta, la plataforma LLAMA POR TELÉFONO al local. Ahí deja de ser
//     un aviso en pantalla y se vuelve un teléfono sonando a media hora pico.
//   - Cerca del final del plazo, la plataforma cancela el pedido ella sola.
//
// Un segundero además obliga a repintar cada segundo una pantalla que también dibuja el catálogo.

export type Urgencia = 'normal' | 'sonando' | 'por_expirar';

/** El plazo total que da la plataforma, en milisegundos. */
const PLAZO_MS = 11.5 * 60 * 1000;
/** Cuándo empieza a sonar el teléfono del local, contado desde que llegó el pedido. */
const ROBOLLAMADA_MS = 90 * 1000;
/** Cuándo se considera que ya casi expira. */
const POR_EXPIRAR_MS = 2 * 60 * 1000;

/**
 * urgenciaDe clasifica un pedido por lo que queda de plazo.
 *
 * `decideBefore` es el instante absoluto que fijó el SERVIDOR, no una duración: el plazo es suyo.
 * La cuenta se hace contra el reloj de la tableta, así que una tableta atrasada promete minutos
 * que ya no existen; por eso los minutos se redondean hacia abajo, y el servidor rechaza decidir
 * un pedido vencido aunque la pantalla todavía lo ofrezca.
 */
export function urgenciaDe(decideBefore: string | null, ahora: number): Urgencia {
  if (!decideBefore) return 'normal';
  const resta = new Date(decideBefore).getTime() - ahora;
  if (Number.isNaN(resta)) return 'normal';
  if (resta <= POR_EXPIRAR_MS) return 'por_expirar';
  if (resta <= PLAZO_MS - ROBOLLAMADA_MS) return 'sonando';
  return 'normal';
}

/**
 * minutosQueQuedan redondea HACIA ABAJO a propósito.
 *
 * Decir «3 minutos» cuando quedan 3 minutos y 50 segundos promete tiempo que no hay, y el costo de
 * esa mentira es un pedido cancelado. Redondear hacia abajo solo puede apurar de más.
 */
export function minutosQueQuedan(decideBefore: string | null, ahora: number): number {
  if (!decideBefore) return 0;
  const resta = new Date(decideBefore).getTime() - ahora;
  if (Number.isNaN(resta) || resta <= 0) return 0;
  return Math.floor(resta / 60000);
}

/**
 * minutosDesde dice hace cuánto llegó un pedido sin plazo (el pedido web). Hacia abajo y nunca
 * negativo: un reloj de tableta atrasado no debe pintar «hace -1 min».
 */
export function minutosDesde(placedAt: string | null, ahora: number): number {
  if (!placedAt) return 0;
  const pasa = ahora - new Date(placedAt).getTime();
  if (Number.isNaN(pasa) || pasa <= 0) return 0;
  return Math.floor(pasa / 60000);
}

/** Cuánto le queda al pedido, o hace cuánto llegó si su origen no tiene plazo. */
export function tiempoDe(p: { decideBefore: string | null; placedAt: string | null }, ahora: number): string {
  return p.decideBefore
    ? `quedan ${minutosQueQuedan(p.decideBefore, ahora)} min`
    : `hace ${minutosDesde(p.placedAt, ahora)} min`;
}
