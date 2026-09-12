// CÓMO SE LLAMA CADA PANTALLA EN LA CONSOLA.
//
// Es una COPIA DELIBERADA de los nombres que el POS manda, no un import: la consola no puede
// importar del POS —`eslint.config.js` lo impide en las dos direcciones— y tampoco debería. Son dos
// productos con dos ciclos de vida: el día que el POS renombre una pantalla, esta lista decide si
// el mapa la sigue llamando igual o no.
//
// El servidor manda slugs (`pos`, `caja`); aquí se decide cómo se leen.
const ETIQUETAS: Record<string, string> = {
  pos: 'Punto de venta',
  pedidos: 'Pedidos en curso',
  caja: 'Caja',
  reportes: 'Reportes',
  ventas: 'Ventas',
  gastos: 'Gastos',
  catalogo: 'Catálogo',
  inventario: 'Inventario',
  usuarios: 'Empleados',
  negocio: 'Ajustes del negocio',
  impresion: 'Impresión',
  cuenta: 'Cuenta propia',
};

const ACCIONES: Record<string, string> = {
  cobrar: 'Cobrar',
  'agregar-producto': 'Agregar producto',
  'cancelar-pedido': 'Cancelar pedido',
  entregar: 'Entregar',
  'abrir-turno': 'Abrir turno',
  'cerrar-turno': 'Cerrar turno',
  'contar-efectivo': 'Contar efectivo',
  traspaso: 'Traspaso',
  'editar-producto': 'Editar producto',
  'crear-producto': 'Crear producto',
  'ajustar-stock': 'Ajustar existencias',
  'registrar-gasto': 'Registrar gasto',
};

// etiquetaDePantalla y etiquetaDeAccion caen al slug si no lo conocen.
//
// Devolver el slug crudo es a propósito: si el POS empieza a mandar un nombre que esta lista no
// tiene, el mapa lo muestra tal cual y se ve raro — que es justo lo que hace que alguien lo
// arregle. Esconderlo lo dejaría sin contar para siempre.
export function etiquetaDePantalla(slug: string): string {
  return ETIQUETAS[slug] ?? slug;
}

export function etiquetaDeAccion(slug: string): string {
  return ACCIONES[slug] ?? slug;
}
