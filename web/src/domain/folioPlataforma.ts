// ¿Hay que pedir el folio antes de mandar este pedido?
//
// Vive en `domain` y no dentro de la pantalla porque es una REGLA —cuándo se interpone la hoja— y
// las reglas se prueban sin montar React. La pantalla la consulta desde los dos caminos que crean
// un pedido (Enviar y Cobrar): un camino nuevo que se salte esta puerta es cómo la mitad de los
// pedidos acabarían sin folio.
export function hayQuePedirElFolio(platformId: number | null, platformOrderRef?: string): boolean {
  // Sin plataforma no hay folio que pedir, y el campo ni siquiera existe en la pantalla.
  if (platformId === null) return false;
  // El parámetro es OPCIONAL y se normaliza aquí, aunque el `merge` del store ya lo rellene al
  // rehidratar. No es donde estaba el defecto —la pantalla en blanco la tiró `FolioPlataformaSheet`,
  // que corre antes que esto— pero una cuenta sin el campo es una entrada que el sistema SÍ produce,
  // y una regla de dominio que revienta con ella está mal escrita.
  //
  // `trim` y no `!== ''`: un campo con puros espacios es el caso típico de "lo toqué y no escribí".
  // El servidor lo rechazaría igual, pero descubrirlo hasta el rechazo cuesta el viaje completo con
  // el repartidor enfrente.
  return (platformOrderRef ?? '').trim() === '';
}
