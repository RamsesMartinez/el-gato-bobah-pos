// Cuándo se pueden reordenar las opciones de un grupo, y qué decirle al operador cuando no.
//
// Reordenar arrastra sobre la lista VISIBLE y guarda ese orden como el real. Si la lista está
// filtrada —por el buscador o porque se ocultaron las archivadas— lo que se ve no es lo que hay, y
// guardar ese orden mandaría al fondo las opciones que no estaban en pantalla.
//
// Vive aparte del componente porque la regla ya se rompió una vez de la forma más cara: al filtrar
// desaparecía el arrastre Y el botón de agregar, sin explicar nada, y el operador lo leyó como que
// la pantalla estaba rota. Un test fija la regla y el mensaje.

export interface EstadoDeOrden {
  puedeReordenar: boolean;
  // Qué hacer para poder reordenar. Vacío cuando ya se puede.
  motivo: string;
}

export function estadoDeOrden(buscando: boolean, archivadasOcultas: number): EstadoDeOrden {
  if (buscando) {
    return { puedeReordenar: false, motivo: 'Limpia el buscador para cambiar el orden.' };
  }
  if (archivadasOcultas > 0) {
    return { puedeReordenar: false, motivo: 'Muestra las archivadas para cambiar el orden.' };
  }
  return { puedeReordenar: true, motivo: '' };
}
