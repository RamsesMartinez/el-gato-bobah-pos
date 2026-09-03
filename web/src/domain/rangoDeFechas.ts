// Reglas del rango libre de fechas, fuera de las pantallas.
//
// Ventas y Reportes ofrecen el mismo control y llaman al mismo servidor, así que las reglas de qué
// rango se puede pedir tienen que ser una sola. Escritas dos veces divergen —ya pasó con el
// redondeo del cobro, que vivía en cinco copias— y el síntoma es peor de lo que suena: una pantalla
// deja pedir lo que la otra rechaza, y el operador no sabe cuál de las dos tiene razón.
//
// Son las MISMAS que `domain.ResolveRange` del servidor. Aquí no sustituyen a las de allá: el
// servidor sigue rechazando lo que no puede atender. Existen para que el rebote se vea ANTES de
// mandar, que es la diferencia entre corregir una fecha y quedarse mirando un error rojo.

// MAX_DIAS_RANGO: espejo de domain.MaxSalesRangeDays. Sin cota, un "del 2020 a hoy" escanea sin
// límite en el gigabyte de RAM del VPS y tumba la API para todos.
export const MAX_DIAS_RANGO = 366;

export type MotivoRangoInvalido =
  | 'incompleto' | 'malformado' | 'invertido' | 'demasiados-dias' | 'en-el-futuro';

// diaValido acepta exactamente lo que el servidor: AAAA-MM-DD y una fecha que existe.
//
// El chequeo del día real importa: `new Date('2026-02-31')` no falla, rueda al 3 de marzo. Un campo
// que acepta el 31 de febrero y consulta el 3 de marzo es la pantalla que se ve bien y contesta otro
// periodo.
export function diaValido(v: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(v)) return false;
  const d = new Date(`${v}T00:00:00Z`);
  if (Number.isNaN(d.getTime())) return false;
  return d.toISOString().slice(0, 10) === v;
}

// diasDelRango cuenta los días del intervalo, INCLUSIVO en los dos extremos: del 1 al 1 es un día.
// Devuelve 0 si alguna fecha no sirve.
export function diasDelRango(desde: string, hasta: string): number {
  if (!diaValido(desde) || !diaValido(hasta)) return 0;
  const a = Date.parse(`${desde}T00:00:00Z`);
  const b = Date.parse(`${hasta}T00:00:00Z`);
  if (b < a) return 0;
  return Math.round((b - a) / 86_400_000) + 1;
}

// validarRango dice por qué un rango no se puede pedir, o null si sí.
//
// `hoy` es el día del NEGOCIO. Va como parámetro y no se lee del reloj aquí dentro para que la
// función siga siendo pura: el día del negocio no es el del navegador, y una tableta con la hora
// corrida decidiría distinto que el servidor.
export function validarRango(desde: string, hasta: string, hoy: string): MotivoRangoInvalido | null {
  if (desde === '' || hasta === '') return 'incompleto';
  if (!diaValido(desde) || !diaValido(hasta)) return 'malformado';
  // Invertido devolvería CERO filas sin error, y el operador creería que no vendió.
  if (Date.parse(`${hasta}T00:00:00Z`) < Date.parse(`${desde}T00:00:00Z`)) return 'invertido';
  if (diasDelRango(desde, hasta) > MAX_DIAS_RANGO) return 'demasiados-dias';
  // El calendario lo topa con `max`, pero `max` no impide teclear la fecha: el navegador solo marca
  // el campo como inválido y aquí no hay validación de formulario que lo frene.
  if (diaValido(hoy) && hasta > hoy) return 'en-el-futuro';
  return null;
}

// mensajeDeRango: qué se le dice a quien opera. Sin nombres de parámetros ni de endpoints — el
// renglón tiene que servirle a quien atiende el negocio, no a quien lo programó.
export function mensajeDeRango(motivo: MotivoRangoInvalido): string {
  switch (motivo) {
    case 'incompleto':
      return 'Elige las dos fechas para ver el periodo.';
    case 'malformado':
      return 'Esa fecha no existe. Revísala.';
    case 'invertido':
      return 'La fecha de inicio va antes que la de fin.';
    case 'demasiados-dias':
      return `El periodo no puede pasar de ${MAX_DIAS_RANGO} días.`;
    case 'en-el-futuro':
      return 'Ese día todavía no llega.';
  }
}
