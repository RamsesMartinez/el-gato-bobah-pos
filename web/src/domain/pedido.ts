import type { ServiceType } from '../types/pos';

// Las reglas del pedido que la pantalla necesita para no contradecir al servidor.
//
// Vive en `domain` —sin React, sin Chakra, sin api— porque es la regla, no la pantalla. Desde la
// 030 el cuerpo del pedido lo arma el servidor a partir de la cuenta en captura; aquí queda solo lo
// que la pantalla tiene que decidir antes de preguntarle.

// cobraEnvio dice si ESTE pedido lleva el costo de envío del negocio.
//
// Una plataforma reparte con su propia gente y cobra ese reparto aparte, así que el envío del
// negocio no aplica aunque la cuenta esté marcada como domicilio — y esa combinación es alcanzable:
// se marca domicilio primero y se asigna la plataforma después.
//
// La regla es la del servidor (`cobraEnvio` en app/orders.go): una segunda deducción es una segunda
// oportunidad de divergir, y cuando divergió la pantalla ofrecía cobrar $115 de un pedido de $95.
export function cobraEnvio(cuenta: { serviceType: ServiceType | string; platformId: number | null }): boolean {
  return cuenta.serviceType === 'domicilio' && cuenta.platformId === null;
}
