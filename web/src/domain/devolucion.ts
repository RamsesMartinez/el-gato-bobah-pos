import { round2 } from './numeros';

// Las reglas de devolver dinero, en la pantalla. Espejo de `domain/devolucion.go`.
//
// No sustituyen a las del servidor: él sigue rechazando lo que no puede atender. Existen para que el
// rebote se vea ANTES de tocar el botón, que es la diferencia entre corregir un monto y quedarse
// mirando un error rojo con el cliente enfrente.

export type MotivoSinDevolucion = 'sin-cobros' | 'ya-devuelto' | 'excede' | 'sin-monto' | 'sin-motivo';

// montoDevolvible: cuánto queda por devolver de lo que ya entró.
//
// Nunca negativo. Si por lo que sea se devolvió de más, lo que queda es cero — no una deuda del
// cliente hacia el negocio, que es lo que un número negativo diría.
export function montoDevolvible(cobrado: number, yaDevuelto: number): number {
  return Math.max(0, round2(cobrado - yaDevuelto));
}

// sePuedeDevolver dice por qué NO se puede devolver, o null si sí.
//
// El tope es lo COBRADO menos lo ya devuelto, nunca el total del pedido: el tablero llegó a ofrecer
// "Reembolsar" junto a "Cobrar $220" en la misma tarjeta, y tocarlo anotaba $220 de pérdida por un
// ingreso que nunca ocurrió.
export function sePuedeDevolver(
  monto: number, cobrado: number, yaDevuelto: number, motivo: string,
): MotivoSinDevolucion | null {
  if (cobrado <= 0) return 'sin-cobros';
  const queda = montoDevolvible(cobrado, yaDevuelto);
  if (queda <= 0) return 'ya-devuelto';
  if (!Number.isFinite(monto) || monto <= 0) return 'sin-monto';
  if (round2(monto) > queda) return 'excede';
  if (motivo.trim() === '') return 'sin-motivo';
  return null;
}

// porQueNoSeDevuelve: qué se le dice a quien opera. Sin nombres de campos ni de endpoints — el
// renglón tiene que servirle a quien atiende el negocio, no a quien lo programó.
export function porQueNoSeDevuelve(motivo: MotivoSinDevolucion): string {
  switch (motivo) {
    case 'sin-cobros':
      return 'Este pedido no se ha cobrado, así que no hay nada que devolver.';
    case 'ya-devuelto':
      return 'Ya se devolvió todo lo que se había cobrado.';
    case 'excede':
      return 'No puedes devolver más de lo que se cobró.';
    case 'sin-monto':
      return 'Escribe cuánto vas a devolver.';
    case 'sin-motivo':
      return 'Escribe por qué se devuelve.';
  }
}

// avisoDeInventario: qué pasa con el insumo al cancelar un renglón.
//
// La regla la decide el servidor con `enviado_a_cocina_at`, y la pantalla la ANUNCIA antes de
// confirmar: cancelar algo que ya salió a cocina baja el total pero NO devuelve el insumo, porque se
// gastó. Callarlo hace que el almacén cuadre mal y nadie sepa por qué.
export function avisoDeInventario(yaSalioACocina: boolean): string {
  return yaSalioACocina
    ? 'Ya se está preparando: se quita de la cuenta, pero el ingrediente no vuelve al almacén.'
    : 'Todavía no se prepara: se quita de la cuenta y el ingrediente vuelve al almacén.';
}
