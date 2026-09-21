import { parseMonto, parseNumero, round2 } from './numeros';

// El descuento de una cuenta, decidido en UN solo lugar.
//
// Existe por lo mismo que `envio.ts`: la pantalla tiene tres superficies que pintan el total —el
// panel del ticket, la píldora flotante y la barra angosta—, y a 1024×600 la que se ve por omisión
// no es el panel. Que cada una restara el descuento por su cuenta es exactamente cómo la pantalla
// llegó a ofrecer cobrar $115 de un pedido de $95.
//
// Lo que se calcula aquí es lo que se PINTA. El monto que se cobra lo resuelve el servidor contra
// su propio subtotal, y por eso el porcentaje viaja como porcentaje: mandarlo ya convertido a pesos
// sería pedirle al servidor que le crea al cliente una cifra de dinero.

export type ModoDeDescuento = 'monto' | 'pct';

export interface DescuentoDeLaCuenta {
  // malEscrito bloquea el envío. Un descuento ilegible que cayera a cero es una promoción que el
  // cliente ya escuchó y que el ticket no aplica.
  malEscrito: boolean;
  // excede: se quiso descontar más de lo que vale la cuenta. Se distingue de malEscrito porque el
  // aviso es otro —cuál es el máximo— y porque el número sí se entendió.
  excede: boolean;
  // monto: lo que se resta del total EN PANTALLA. Con un valor inválido es 0, y lo que impide
  // cobrar ese cero es el botón apagado.
  monto: number;
  // paraElServidor: qué mandar en el cuerpo del pedido. `undefined` = no hay descuento que mandar
  // (ausente, ilegible o imposible), y entonces el campo ni siquiera viaja.
  paraElServidor: { discountAmount: number } | { discountPercent: number } | undefined;
}

const SIN_DESCUENTO: DescuentoDeLaCuenta = {
  malEscrito: false, excede: false, monto: 0, paraElServidor: undefined,
};

export function descuentoDeLaCuenta(
  capturado: string,
  modo: ModoDeDescuento,
  subtotal: number,
): DescuentoDeLaCuenta {
  // El porcentaje NO es dinero: se lee con parseNumero para no redondearlo a centavos antes de
  // aplicarlo.
  const n = modo === 'pct' ? parseNumero(capturado) : parseMonto(capturado);
  if (n.estado === 'ausente') return SIN_DESCUENTO;
  if (n.estado === 'invalido') {
    return { malEscrito: true, excede: false, monto: 0, paraElServidor: undefined };
  }

  if (modo === 'pct') {
    if (n.valor > 100) {
      return { malEscrito: false, excede: true, monto: 0, paraElServidor: undefined };
    }
    return {
      malEscrito: false, excede: false,
      monto: round2((subtotal * n.valor) / 100),
      paraElServidor: { discountPercent: n.valor },
    };
  }

  if (n.valor > subtotal) {
    return { malEscrito: false, excede: true, monto: 0, paraElServidor: undefined };
  }
  return {
    malEscrito: false, excede: false, monto: n.valor,
    paraElServidor: { discountAmount: n.valor },
  };
}

// totalConDescuento arma el total de la cuenta en el ORDEN que manda: se descuenta la comida y el
// envío se suma después.
//
// Al revés, una promoción del menú le recortaría al repartidor lo que se le paga por llevarla. El
// piso en cero es el mismo que aplica el servidor: cancelar renglones puede dejar el subtotal por
// debajo de un descuento ya capturado, y un total negativo no se cobra, se devuelve.
export function totalConDescuento(subtotal: number, descuento: number, envio: number): number {
  return round2(Math.max(round2(subtotal - descuento), 0) + envio);
}
