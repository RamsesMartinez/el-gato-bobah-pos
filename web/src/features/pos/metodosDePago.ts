import type { PaymentMethod } from '../../types/pos';

// Cómo la pantalla de cobro decide qué método usar, sin depender de ningún id fijo.
//
// Antes esta lógica eran cuatro ids quemados (1, 2, 3, 7) tomados de los seeds. Funcionaba mientras
// payment_methods fue una tabla GLOBAL con esos ids; desde que es per-tenant, cada empresa tiene
// los suyos y una empresa que no sea la primera recibe ids distintos: ningún botón quedaba marcado
// y el panel de "recibido y cambio" nunca aparecía, porque su efectivo ya no era el id 1.
//
// Vive fuera del componente para que la regla tenga test propio: es la clase de fallo que solo se
// nota con un segundo cliente, y para entonces ya está en producción.

// esEfectivo dice si un método mueve billetes físicos. Es lo que enciende el panel de recibido y
// cambio, y lo decide el `kind` que manda el servidor, no el nombre ni el id.
export function esEfectivo(m: PaymentMethod | undefined): boolean {
  return m?.kind === 'efectivo';
}

// metodoPorDefecto: con qué método abre el cobro. Se elige el primero que NO sea efectivo, porque
// el efectivo obliga a capturar lo recibido y arrancar ahí le cuesta un paso al caso más común.
// Si el negocio solo cobra en efectivo, cae al primero que haya en vez de dejar la pantalla sin
// método seleccionado.
export function metodoPorDefecto(metodos: PaymentMethod[]): number | null {
  if (metodos.length === 0) return null;
  return (metodos.find((m) => m.kind !== 'efectivo') ?? metodos[0]).id;
}

// primerMetodoLibre: para el pago dividido, el primer método que todavía no se usó en otra línea.
// Repetir método en dos líneas no aporta y confunde el corte.
export function primerMetodoLibre(metodos: PaymentMethod[], usados: number[]): number | null {
  if (metodos.length === 0) return null;
  const yaUsados = new Set(usados);
  return (metodos.find((m) => !yaUsados.has(m.id)) ?? metodos[0]).id;
}
