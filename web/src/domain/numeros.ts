// Leer un número que tecleó una persona, y redondearlo. Una sola implementación, con su prueba.
//
// Antes de este archivo había CUATRO redondeos a dos decimales en el front —dos de ellos sin el
// EPSILON— y dos formas distintas de leer el mismo campo. Ninguna copia la escribió alguien
// descuidado: cada una nació porque quien la necesitaba no sabía que la otra existía, y nada falló
// al escribirla. La regla de lint que prohíbe `Math.round(x*100)/100` y `parseFloat` fuera de aquí
// es lo que impide la quinta.

// EPSILON: 1.005 no es exacto en binario, así que `1.005 * 100` da 100.49999999999999 y Math.round
// lo baja a 1.00. Un peso que baja medio centavo no se nota en una venta y no se explica en un corte.
function redondea(n: number, decimales: number): number {
  const factor = 10 ** decimales;
  return Math.round((n + Number.EPSILON) * factor) / factor;
}

// round2 es la escala del DINERO: es la de las columnas `numeric(10,2)` del servidor.
export function round2(n: number): number {
  return redondea(n, 2);
}

// round4 es la escala del STOCK: `numeric(14,4)`. Redondear el inventario a dos decimales corrompe
// lo que se vende por gramo.
export function round4(n: number): number {
  return redondea(n, 4);
}

export type Numero =
  | { estado: 'ausente' }
  | { estado: 'valido'; valor: number }
  | { estado: 'invalido'; motivo: 'formato' | 'negativo' };

// Solo dígitos, con un punto decimal opcional. Sin comas, sin cola, sin notación científica.
const SOLO_NUMERO = /^-?\d+(\.\d+)?$/;

// parseNumero distingue las TRES cosas que un campo puede ser, y esa es toda su razón de existir.
//
// AUSENTE no es cero. Un campo vacío significa "no lo escribí", que en el cobro quiere decir "pagó
// justo": tratarlo como cero deja el aviso de faltante encendido para siempre.
//
// INVÁLIDO tampoco es cero, y aquí está el caso que más caro sale: `parseFloat('1,000')` devuelve 1,
// y es un número finito, así que ni siquiera las guardas de `Number.isFinite` lo atrapan. El operador
// teclea la coma de millar por costumbre y cobra $999 de menos. `parseFloat('12kg')` devuelve 12 y
// tira la unidad. Se rechaza en vez de adivinar: no hay forma de saber si "1,000" quiso decir mil o
// uno, y adivinar mal mueve dinero.
export function parseNumero(texto: string): Numero {
  const s = texto.trim();
  if (s === '') return { estado: 'ausente' };
  if (!SOLO_NUMERO.test(s)) return { estado: 'invalido', motivo: 'formato' };
  const n = Number(s);
  if (!Number.isFinite(n)) return { estado: 'invalido', motivo: 'formato' };
  if (n < 0) return { estado: 'invalido', motivo: 'negativo' };
  return { estado: 'valido', valor: n };
}

// parseMonto es parseNumero a la escala del dinero. Va aparte porque redondear a dos decimales una
// cantidad de stock la corrompe, y ese error es invisible hasta el inventario.
export function parseMonto(texto: string): Numero {
  const n = parseNumero(texto);
  return n.estado === 'valido' ? { estado: 'valido', valor: round2(n.valor) } : n;
}

// montoTecleado lee un campo de DINERO que escribió una persona, y devuelve undefined si lo que
// escribió no es un monto.
//
// Reemplaza al patrón `parseFloat(x) || 0`, que estaba en veinticinco lugares y en todos hacía lo
// mismo: convertir un error de captura en un cero silencioso. En el fondo con el que abre la caja
// eso significa arrancar el turno con $0 declarados y cerrar con un sobrante del monto entero; en un
// precio, guardar un producto en cero; en el costo de envío, envío gratis. Nada de eso avisa.
//
// Quien llama decide qué hace con el undefined —apagar el botón, avisar— pero ya no puede ignorarlo
// por accidente, porque `undefined` no se suma.
export function montoTecleado(texto: string): number | undefined {
  const n = parseMonto(texto);
  return n.estado === 'valido' ? n.valor : undefined;
}

// valorODefault saca el número de un campo aceptando el default SOLO cuando está ausente.
//
// El principio V lo dice con estas palabras: el default es para el parámetro AUSENTE, nunca para el
// presente y malformado. Un envío escrito "1,000" que cae a $0 es envío gratis sin que nadie lo
// decida; un `undefined` obliga a quien llama a decidir qué hacer con el error.
export function valorODefault(texto: string, siAusente: number): number | undefined {
  const n = parseNumero(texto);
  if (n.estado === 'ausente') return siAusente;
  return n.estado === 'valido' ? n.valor : undefined;
}
