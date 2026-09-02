// La aritmética de dinero del cobro, fuera de las pantallas.
//
// Vivía inline en las dos hojas de cobro —`round2` en cinco copias, dos parsers distintos del mismo
// campo, y ni un solo test— y por eso divergieron: una filtraba los billetes contra lo que falta y
// la otra los pintaba todos, una avisaba de efectivo insuficiente y la otra no. Aquí es una sola
// implementación con su prueba, que es lo que el principio IV pide de la lógica pura.
//
// UN COBRO A LA VEZ, y esa es la decisión de diseño que da forma a todo lo demás. Una hoja que
// captura N pagos y los manda de un golpe registra dinero que todavía no se recibió: si la terminal
// declina la tarjeta del segundo comensal DESPUÉS del acuse, el sistema ya lo dio por cobrado y no
// existe forma de deshacer un pago — no hay endpoint que lo quite, y el reembolso es de la cuenta
// entera. Cobrando de a un pedazo, el registro coincide con el instante en que el dinero está en la
// mano, y lo que falta lo dice el servidor entre uno y otro en vez de una resta local.

// Billetes MXN que el operador toca en vez de teclear.
const BILLETES = [50, 100, 200, 500, 1000];

export function round2(n: number): number {
  // El +Number.EPSILON corrige el caso clásico de los binarios: 1.005*100 es 100.49999999999999 y
  // Math.round lo baja a 1.00, un centavo que en un corte de caja no se explica.
  return Math.round((n + Number.EPSILON) * 100) / 100;
}

export type Monto =
  | { estado: 'ausente' }
  | { estado: 'valido'; valor: number }
  | { estado: 'invalido'; motivo: 'formato' | 'negativo' };

// parseMonto distingue las TRES cosas que un campo de dinero puede ser, y esa es toda su razón de
// existir.
//
// - AUSENTE no es cero. El campo de "con cuánto paga" vacío significa "pagó justo", y tratarlo como
//   $0 recibidos deja el aviso de efectivo insuficiente encendido para siempre: el botón de cobrar
//   muerto en el caso más común.
// - INVÁLIDO no es cero. `Number('abc')` da NaN y se disolvía en un cambio de $0 con el botón
//   todavía cobrable; `parseFloat('1,000')` da 1, así que la coma de millar que el operador teclea
//   por costumbre cobraba $999 de menos. Se rechaza en vez de adivinar: no hay forma de saber si
//   "1,000" quiso decir mil o uno.
export function parseMonto(texto: string): Monto {
  const s = texto.trim();
  if (s === '') return { estado: 'ausente' };
  if (!/^-?\d+(\.\d+)?$/.test(s)) return { estado: 'invalido', motivo: 'formato' };
  const n = Number(s);
  if (!Number.isFinite(n)) return { estado: 'invalido', motivo: 'formato' };
  if (n < 0) return { estado: 'invalido', motivo: 'negativo' };
  return { estado: 'valido', valor: round2(n) };
}

// dividirEnPartes reparte un monto en N, con el RESIDUO en la última.
//
// `total/n` redondeado no suma el total: $100 en tres da tres partes de $33.33 = $99.99. El servidor
// tolera ese centavo y CIERRA el pedido, así que quedaba un centavo que nadie podía cobrar y que la
// barra del POS seguía sumando — el pedido salía de la vista al día siguiente con la deuda abierta.
//
// Devuelve null cuando alguna parte quedaría en cero: cobrar $0 no es cobrar.
export function dividirEnPartes(total: number, n: number): number[] | null {
  if (n < 1 || !Number.isInteger(n)) return null;
  const parte = Math.floor(round2(total) * 100 / n) / 100;
  if (parte <= 0) return null;
  const partes = Array.from({ length: n }, () => parte);
  partes[n - 1] = round2(round2(total) - parte * (n - 1));
  return partes;
}

export interface Sugerencia {
  etiqueta: string;
  monto: number;
}

// sugerenciasDeMonto son los atajos de "¿cuánto cobras ahora?": todo, o entre cuántos se reparte.
//
// Existen para que dividir NO cueste teclear. El teclado del sistema come 250 de los 600 px de alto
// de la tableta y tapa justo la cifra que decide si el botón se enciende; un preset de un toque
// evita abrirlo en el caso común, que es repartir parejo entre dos, tres o cuatro.
//
// La parte que se ofrece es la PRIMERA de la división, no la última: la última se lleva el residuo y
// se cobra sola cuando el faltante ya es exactamente ella.
export function sugerenciasDeMonto(falta: number): Sugerencia[] {
  const out: Sugerencia[] = [{ etiqueta: 'Todo', monto: round2(falta) }];
  for (const n of [2, 3, 4]) {
    const partes = dividirEnPartes(falta, n);
    // Sin la guarda, repartir $0.02 entre tres ofrece partes de $0.00 y el cobro rebota.
    if (partes) out.push({ etiqueta: `Entre ${n}`, monto: partes[0] });
  }
  return out;
}

export type MotivoInvalido =
  | 'sin-monto' | 'monto-invalido' | 'sin-metodo' | 'excede' | 'propina-excede' | 'falta-efectivo';

export interface Veredicto {
  ok: boolean;
  motivo?: MotivoInvalido;
  monto: number;
  propina: number;
  // cambio y falta son del efectivo recibido: cuánto se devuelve y cuánto falta para completar.
  cambio: number;
  faltaEfectivo: number;
}

export interface Entrada {
  // monto: lo que se cobra AHORA. Puede ser menos que el faltante: eso es dividir la cuenta.
  monto: string;
  metodoId: number | null;
  propina: string;
  // recibido: con cuánto paga, solo para efectivo. '' significa "pagó justo".
  recibido: string;
  esEfectivo: boolean;
  // falta: lo que el SERVIDOR dice que queda del pedido. Nunca una resta local — es la única cifra
  // que sobrevive a que otra caja cobre el mismo pedido mientras esta hoja está abierta.
  falta: number;
  // totalDelPedido topa la propina, con el mismo predicado que domain.ValidarPropina.
  totalDelPedido: number;
}

// validarCobro decide si el botón se enciende, y por qué no cuando no.
export function validarCobro(e: Entrada): Veredicto {
  const vacio: Veredicto = { ok: false, monto: 0, propina: 0, cambio: 0, faltaEfectivo: 0 };

  const m = parseMonto(e.monto);
  if (m.estado === 'ausente') return { ...vacio, motivo: 'sin-monto' };
  if (m.estado === 'invalido') return { ...vacio, motivo: 'monto-invalido' };
  if (m.valor === 0) return { ...vacio, motivo: 'sin-monto' };

  const p = parseMonto(e.propina);
  if (p.estado === 'invalido') return { ...vacio, monto: m.valor, motivo: 'monto-invalido' };
  const propina = p.estado === 'valido' ? p.valor : 0;

  const base: Veredicto = { ok: false, monto: m.valor, propina, cambio: 0, faltaEfectivo: 0 };

  // El exceso se ve ANTES de mandar. Si no, el cobro sale, el servidor lo rechaza con ErrCobroExcede
  // y el operador se entera con el dinero del cliente en la mano.
  //
  // El tope es EXACTO, como el de domain.ValidarCobro: el centavo de tolerancia vive en el predicado
  // que CIERRA el pedido, no en el que acota cada cobro. El round2 es contra el ruido de los
  // flotantes, no una holgura: 33.34 - 33.33 da 0.010000000000001563 en binario.
  if (round2(m.valor - e.falta) > 0) return { ...base, motivo: 'excede' };
  if (e.metodoId === null) return { ...base, motivo: 'sin-metodo' };
  if (!propinaValida(propina, e.totalDelPedido)) return { ...base, motivo: 'propina-excede' };

  if (!e.esEfectivo) return { ...base, ok: true };

  const c = cambioDeEfectivo(e.recibido, round2(m.valor + propina));
  if (c.invalido) return { ...base, motivo: 'monto-invalido' };
  // El faltante de efectivo es el único control que impide cobrar de menos, y en modo dividido no
  // existía en ninguna de las dos hojas: la línea de efectivo —la más común en el mostrador— se
  // quedaba sin él.
  if (c.falta > 0) return { ...base, motivo: 'falta-efectivo', faltaEfectivo: c.falta };
  return { ...base, ok: true, cambio: c.cambio };
}

export interface Cambio {
  exacto: boolean;
  cambio: number;
  falta: number;
  invalido?: boolean;
}

// cambioDeEfectivo calcula el vuelto y el faltante de un cobro en efectivo.
export function cambioDeEfectivo(recibido: string, aCubrir: number): Cambio {
  const m = parseMonto(recibido);
  if (m.estado === 'ausente') return { exacto: true, cambio: 0, falta: 0 };
  if (m.estado === 'invalido') return { exacto: false, cambio: 0, falta: 0, invalido: true };
  const diff = round2(m.valor - aCubrir);
  return { exacto: false, cambio: Math.max(0, diff), falta: Math.max(0, -diff) };
}

// billetesUtiles deja solo los billetes con los que el cliente alcanza a pagar.
//
// Pintarlos todos deja tocar un $50 sobre $175: la pantalla entra en "falta efectivo" y deshabilita
// el cobro. Un control que solo sirve para trabar la pantalla es peor que no tenerlo.
export function billetesUtiles(monto: number): number[] {
  return BILLETES.filter((b) => b >= monto);
}

// propinaValida es el espejo de domain.ValidarPropina: el rebote se ve antes de tener el dinero del
// cliente en la mano, no después. El tope es la cuenta entera y va por pago, igual que el servidor.
export function propinaValida(propina: number, totalDelPedido: number): boolean {
  return propina >= 0 && round2(propina) <= round2(totalDelPedido);
}

// presetsDePropina calcula los porcentajes sobre LO QUE SE COBRA AHORA, no sobre el total del pedido.
//
// Sobre el total, un pedido de $500 con $300 ya abonados ofrecía "15%" = $75 sobre una pantalla que
// dice Cobrar $200: 37.5% de la cifra que el operador tiene enfrente, sin error que lo delate porque
// pasa el tope. El comensal que está pagando deja propina por lo suyo.
export function presetsDePropina(montoACobrar: number): Array<{ etiqueta: string; monto: number }> {
  return [10, 15, 20].map((pct) => ({ etiqueta: `${pct}%`, monto: round2(montoACobrar * pct / 100) }));
}
