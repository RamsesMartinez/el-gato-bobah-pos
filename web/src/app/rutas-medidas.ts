// QUÉ PANTALLA ES CADA RUTA (spec 017).
//
// El nombre que viaja al servidor es el de esta lista, no el path: los paths cambian —`/productos`
// ya es un redirect a `/catalogo/productos`— y si el nombre cambiara con ellos, el historial del
// mapa se partiría en dos pantallas que en realidad son la misma.
//
// **Esta lista tiene que coincidir con la de `server/internal/domain/uso.go`.** Lo que no esté allá
// se descarta en el servidor y queda contado en su log: el mapa mostraría un cero permanente, que
// se lee como «nadie la usa» en vez de como «está mal escrita».
const PANTALLAS: Array<[RegExp, string]> = [
  [/^\/pos/, 'pos'],
  [/^\/pedidos/, 'pedidos'],
  [/^\/caja/, 'caja'],
  [/^\/reportes/, 'reportes'],
  [/^\/ventas/, 'ventas'],
  [/^\/gastos/, 'gastos'],
  // Las tres pestañas del catálogo son la misma pantalla para esta medición: lo que se quiere
  // saber es si se entra al catálogo, no en qué pestaña se quedó. El día que importe, son tres
  // entradas y ninguna migración.
  [/^\/catalogo/, 'catalogo'],
  [/^\/almacen/, 'inventario'],
  [/^\/empleados/, 'usuarios'],
  [/^\/negocio/, 'negocio'],
  [/^\/impresion/, 'impresion'],
  [/^\/cuenta/, 'cuenta'],
];

// pantallaDe traduce una ruta al nombre que se mide, o `null` si esa ruta no se mide.
//
// Devolver `null` es deliberado y no un hueco: el login, la recuperación de contraseña y el reset
// **no se pueden medir** —sin sesión el servidor no sabe de qué empresa ni de qué rol es— y
// inventarles un nombre solo llenaría el log de descartes.
export function pantallaDe(ruta: string): string | null {
  for (const [patron, nombre] of PANTALLAS) {
    if (patron.test(ruta)) return nombre;
  }
  return null;
}

// QUÉ PANTALLAS MIDEN EL TOQUE (spec 019).
//
// Lista corta a propósito: se empieza por el POS, que es donde el dedo está todo el día. Medir doce
// pantallas para mirar dos es volumen y ruido, y la 017 es la que va a decir cuál agregar.
//
// **Tiene que coincidir con `pantallasConToque` en `server/internal/domain/toque.go`**, y esa a su
// vez es subconjunto de la lista de la 017. Una pantalla que esté aquí y no allá manda toques todo
// el turno para que el servidor los tire; una que esté allá y no aquí sale con la rejilla vacía, y
// una rejilla vacía se lee como «aquí nadie toca».
const PANTALLAS_CON_TOQUE = new Set(['pos']);

// seMidenSusToques filtra EN LA TABLETA lo que el servidor iba a descartar de todos modos.
//
// El escuchador vive en la raíz y ve toda la aplicación: sin esto, encolaría toques de pantallas no
// instrumentadas durante todo el turno —y el wifi del mostrador es el mismo por el que viaja un
// cobro—.
export function seMidenSusToques(pantalla: string | null): boolean {
  return pantalla !== null && PANTALLAS_CON_TOQUE.has(pantalla);
}
