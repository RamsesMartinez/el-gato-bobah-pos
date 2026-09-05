import { tituloDeEntregadas, vacioDeEntregadas } from './ventanaDeEntregadas';

// EL ENCABEZADO NOMBRA LA VENTANA QUE LA LISTA CONTIENE.
//
// Decía "Entregadas hoy" siempre, aunque el negocio hubiera elegido que la lista se vacíe al abrir
// el siguiente turno o al cerrar la caja. Un rótulo que nombra un periodo distinto del que se está
// mostrando manda a concluir sobre algo que nadie consultó — es la misma familia de defecto que ya
// costó un turno con $4,500 sin explicar.
test('el título dice la ventana que el negocio configuró', () => {
  expect(tituloDeEntregadas('medianoche')).toBe('Entregadas hoy');
  expect(tituloDeEntregadas('turno')).toBe('Entregadas en este turno');
  expect(tituloDeEntregadas('cierre_de_caja')).toBe('Entregadas desde el último corte');
});

// El vacío sale de la MISMA ventana que el título. Si divergen, la pantalla se contradice consigo
// misma en dos renglones seguidos.
test('el mensaje de lista vacía usa la misma ventana que el título', () => {
  for (const corte of ['medianoche', 'turno', 'cierre_de_caja']) {
    const titulo = tituloDeEntregadas(corte);
    const vacio = vacioDeEntregadas(corte);
    // Los dos terminan nombrando el mismo periodo.
    const periodo = titulo.replace('Entregadas ', '');
    expect(vacio, `"${titulo}" y "${vacio}" hablan de periodos distintos`)
      .toBe(`Sin entregas ${periodo}`);
  }
});

// Un modo desconocido —un ajuste viejo, un valor metido por fuera— cae al default del producto y
// nunca a un rótulo vacío ni al nombre interno del modo. La pantalla no puede quedarse sin título.
test('un modo desconocido cae al default y nunca deja el rótulo vacío', () => {
  for (const raro of [undefined, '', 'por_luna_llena']) {
    expect(tituloDeEntregadas(raro)).toBe('Entregadas hoy');
    expect(vacioDeEntregadas(raro)).toBe('Sin entregas hoy');
  }
});
