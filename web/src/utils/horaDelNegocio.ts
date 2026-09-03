import { DEFAULT_TIMEZONE } from './zonaPorDefecto';

// Formatear fecha y hora EN LA ZONA DEL NEGOCIO, y en un solo lugar.
//
// Antes cada pantalla llamaba a `toLocaleString` por su cuenta, sin zona, así que todas decían la
// hora del navegador de esa tableta: dos Surface con el reloj distinto mostraban horas distintas del
// mismo pedido, y el ticket que se lleva el cliente llevaba la hora de la máquina. Once sitios
// sueltos es exactamente cómo eso se desincronizó; que haya uno solo es lo que impide que vuelva.
//
// Son funciones puras y no un hook: así las usan también los dos papeles, que se arman fuera de
// React.

// zonaSegura devuelve una zona que el navegador acepta.
//
// Una zona que ya no existe —la base de zonas cambia, un nombre se retira— no puede tumbar la
// pantalla ni caer a UTC en silencio, que correría la hora seis horas y parecería plausible. Se cae
// al default del producto, que es el que se parece a la verdad.
export function zonaSegura(zona: string | undefined): string {
  if (!zona) return DEFAULT_TIMEZONE;
  try {
    new Intl.DateTimeFormat('es-MX', { timeZone: zona });
    return zona;
  } catch {
    return DEFAULT_TIMEZONE;
  }
}

// zonaEsUsable dice si el navegador reconoce la zona. Lo usa quien tenga que dejar constancia de
// que un negocio tiene la zona rota: sin ese aviso, la pantalla se comporta bien y nadie se entera.
export function zonaEsUsable(zona: string | undefined): boolean {
  if (!zona) return false;
  try {
    new Intl.DateTimeFormat('es-MX', { timeZone: zona });
    return true;
  } catch {
    return false;
  }
}

// fechaYHora: "1 sep 2026, 20:25" en la zona del negocio.
export function fechaYHora(iso: string | Date | null | undefined, zona: string): string {
  const d = aFecha(iso);
  if (!d) return '';
  return d.toLocaleString('es-MX', {
    timeZone: zonaSegura(zona), dateStyle: 'medium', timeStyle: 'short',
  });
}

// soloHora: "20:25" en la zona del negocio.
export function soloHora(iso: string | Date | null | undefined, zona: string): string {
  const d = aFecha(iso);
  if (!d) return '';
  return d.toLocaleTimeString('es-MX', {
    timeZone: zonaSegura(zona), hour: '2-digit', minute: '2-digit',
  });
}

// soloFecha: "1 sep 2026" en la zona del negocio.
export function soloFecha(iso: string | Date | null | undefined, zona: string): string {
  const d = aFecha(iso);
  if (!d) return '';
  return d.toLocaleDateString('es-MX', { timeZone: zonaSegura(zona), dateStyle: 'medium' });
}

// diaDelNegocio: el día en formato AAAA-MM-DD, en la zona del NEGOCIO.
//
// Es el formato con el que se habla con el servidor y con el que un `input type="date"` acota lo
// que se puede elegir, así que no puede salir de `toLocaleDateString` con un formato legible.
//
// La zona importa aquí tanto como en la hora: a las 19:00 de México ya es el día siguiente en UTC,
// así que un tope calculado con el reloj del navegador en UTC dejaría elegir mañana, y un rango que
// incluye un día que no ha pasado devuelve una pantalla vacía que se lee como "no vendimos nada".
// `en-CA` es el truco estándar para sacar ISO de Intl sin armar la cadena a mano.
export function diaDelNegocio(v: string | Date | null | undefined, zona: string): string {
  const d = aFecha(v);
  if (!d) return '';
  return d.toLocaleDateString('en-CA', { timeZone: zonaSegura(zona) });
}

// aFecha acepta lo que las pantallas tienen a la mano y devuelve null si no es una fecha.
// Una fecha inválida se pinta como vacío, no como "Invalid Date".
function aFecha(v: string | Date | null | undefined): Date | null {
  if (!v) return null;
  const d = v instanceof Date ? v : new Date(v);
  return Number.isNaN(d.getTime()) ? null : d;
}
