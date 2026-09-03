import { parseMonto } from './numeros';
import { cobraEnvio } from './pedido';
import type { TicketTab } from '../types/pos';

// El costo de envío de una cuenta, decidido en UN solo lugar.
//
// Estaba repartido: el panel del ticket calculaba lo suyo y apagaba sus botones con un valor mal
// escrito, mientras la píldora y la barra angosta —los dos caminos con los que se cobra cuando el
// panel está oculto, que en 1024×600 es el default— no lo checaban y mandaban el pedido con el
// envío POR DEFECTO del negocio. Teclear "1,000" (la coma es el separador natural en es-MX y el
// teclado decimal de Android la ofrece) y cobrar desde la píldora cobraba $20.
//
// Y el total tampoco coincidía: el panel pintaba total + envío y la píldora pintaba el total pelón,
// del mismo pedido. Es la forma exacta del defecto que `pedido.ts` documenta —la pantalla ofrecía
// cobrar $115 de un pedido de $95— y el corolario del principio III que prohíbe que la lista y el
// resumen de una pantalla salgan de predicados distintos.

export interface EnvioDeLaCuenta {
  // aplica: si este pedido lo cobra el negocio. Con plataforma lo cobra ella.
  aplica: boolean;
  // malEscrito bloquea el cobro. SOLO cuando el envío aplica: si no, el valor capturado no
  // significa nada y no puede trabar una venta de mostrador sin dejar campo que corregir.
  malEscrito: boolean;
  // monto: lo que suma al total EN PANTALLA. Mal escrito suma cero, y el botón apagado es lo que
  // impide cobrar ese cero — un envío que cae a cero en silencio es envío gratis que nadie decidió.
  monto: number;
  // paraElServidor: qué mandar en `deliveryFee`. `undefined` = no capturado, que el servidor
  // resuelve con el default del negocio. Nunca se manda un valor mal escrito.
  paraElServidor: number | undefined;
}

export function envioDeLaCuenta(
  cuenta: Pick<TicketTab, 'serviceType' | 'platformId'>,
  capturado: string,
  porDefecto: number,
): EnvioDeLaCuenta {
  const aplica = cobraEnvio(cuenta);
  if (!aplica) {
    return { aplica: false, malEscrito: false, monto: 0, paraElServidor: undefined };
  }
  const m = parseMonto(capturado);
  if (m.estado === 'invalido') {
    return { aplica: true, malEscrito: true, monto: 0, paraElServidor: undefined };
  }
  if (m.estado === 'ausente') {
    return { aplica: true, malEscrito: false, monto: porDefecto, paraElServidor: undefined };
  }
  return { aplica: true, malEscrito: false, monto: m.valor, paraElServidor: m.valor };
}
