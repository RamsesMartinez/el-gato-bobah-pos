// nombreLibre elige el animal con el que se va a cantar un pedido.
//
// La LISTA no vive aquí: la sirve el servidor en /pos/folio-names, que es su única copia. Tenerla
// también en el front hacía que agregar un animal de un lado dejara al otro sin poder mostrarlo o
// sin poder repartirlo, y esa divergencia solo se habría notado en el papel.
//
// Solo evita chocar con las cuentas que la pantalla tiene abiertas; los pedidos ya cobrados del
// día no los conoce. Ese choque lo resuelve el servidor agregando la vuelta ("Tigre 2"), que
// conserva el animal —quien pidió ya lo oyó— en vez de saltar a otro.
export function nombreLibre(animales: string[], usados: string[], azar: () => number = Math.random): string {
  if (animales.length === 0) return '';
  const tomados = new Set(usados);
  const libres = animales.filter((a) => !tomados.has(a));
  // Con todos los animales tomados a la vez —haría falta una cuenta abierta por cada uno— se
  // repite uno y el servidor lo desempata.
  const pool = libres.length > 0 ? libres : animales;
  return pool[Math.floor(azar() * pool.length)];
}
