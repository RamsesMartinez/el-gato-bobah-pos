// nombreLibre elige el nombre con el que se va a cantar un pedido.
//
// La LISTA no vive aquí: la sirve el servidor en /pos/folio-names, que además ya no manda la lista
// completa sino LO QUE QUEDA en la bolsa del negocio —y en el esquema que ese negocio eligió, razas
// de gato o animales—. Tener una copia en el front dejaría a la pantalla proponiendo nombres ya
// consumidos, y el servidor los cambiaría al confirmar: justo el nombre que el operador ya le dijo
// al cliente.
//
// Solo evita chocar con las cuentas que la pantalla tiene abiertas; los pedidos ya cobrados del día
// no los conoce. Ese choque lo resuelve el servidor, y desde la bolsa lo resuelve SALTANDO A OTRO
// nombre: mientras quede uno libre, otro nombre es mejor que "Persa 2" con el primer Persa todavía
// en la plancha. El sufijo numerado quedó como última red, para el día que pase del largo de la
// lista.
export function nombreLibre(disponibles: string[], usados: string[], azar: () => number = Math.random): string {
  if (disponibles.length === 0) return '';
  const tomados = new Set(usados);
  const libres = disponibles.filter((a) => !tomados.has(a));
  // Con todo lo disponible tomado a la vez —haría falta una cuenta abierta por cada nombre— se
  // repite uno y el servidor lo desempata.
  const pool = libres.length > 0 ? libres : disponibles;
  return pool[Math.floor(azar() * pool.length)];
}
