// Cómo se nombran en pantalla los valores que la base guarda en inglés-de-esquema.
//
// Viven aparte del componente para tener test propio: son la clase de cosa que alguien agrega al
// enum del backend y se olvida de traducir, y entonces la pantalla muestra `para_llevar` con guion
// bajo a quien opera el negocio.

const ESTADOS: Record<string, string> = {
  abierta: 'Abierta',
  lista: 'Lista',
  entregada: 'Entregada',
  cancelada: 'Cancelada',
  reembolsada: 'Reembolsada',
};

const TIPOS: Record<string, string> = {
  mostrador: 'Mostrador',
  para_llevar: 'Para llevar',
  domicilio: 'Domicilio',
};

// El valor crudo como fallback y no una cadena vacía: un estado nuevo sin traducir se ve feo, pero
// una celda vacía esconde que la venta existe.
export function etiquetaEstado(s: string): string {
  return ESTADOS[s] ?? s;
}

export function etiquetaTipo(s: string): string {
  return TIPOS[s] ?? s;
}
