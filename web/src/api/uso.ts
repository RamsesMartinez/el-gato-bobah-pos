import { useSessionStore } from '../stores/session';

// MEDIR QUÉ SE USA DEL SISTEMA (spec 017), sin que el operador se entere.
//
// La regla que ordena todo este archivo: **nada de aquí puede estar en el camino de un toque**. No
// se espera ninguna respuesta, no se reintenta, no se guarda nada en disco y si algo falla se
// pierde en silencio. Entre perder una medición y estorbar un cobro, se pierde la medición — lo
// dice el spec y lo exige la constitución en sus restricciones de producto.

const BASE = import.meta.env.VITE_API_URL || '/api/v1';

// Cuándo se manda: al juntar 20, cada 10 segundos, o cuando la pestaña se va.
const TOPE_DEL_LOTE = 20;
const CADA_MS = 10_000;

// Lo que cabe esperando mientras un envío está en vuelo. Más allá, se tira lo más VIEJO: si el
// wifi está mal, lo último que hizo el operador dice más que lo de hace cinco minutos.
const MAX_EN_COLA = 50;

interface EventoDeUso {
  pantalla: string;
  accion?: string;
}

// Un toque por zona (spec 019). La celda ya viene calculada: aquí nunca hay un (x, y), ni siquiera
// de paso — ver `celda.ts`.
interface ToqueDeUso {
  pantalla: string;
  celda: number;
  orientacion: 'horizontal' | 'vertical';
}

// UNA SOLA COLA PARA LAS DOS COSAS, y se separan al mandar.
//
// Dos colas serían dos relojes, dos guardas de «uno en vuelo» y dos peticiones cada diez segundos
// compitiendo por la misma red flaky con el POST del cobro. Todo lo que hace que esta medición no
// estorbe está aquí y se comparte.
let cola: Array<EventoDeUso | ToqueDeUso> = [];
let enVuelo = false;
let reloj: ReturnType<typeof setInterval> | null = null;

// medirPantalla cuenta que alguien ABRIÓ una pantalla.
export function medirPantalla(pantalla: string): void {
  encolar({ pantalla });
}

// medirAccion cuenta que alguien disparó una acción con nombre dentro de una pantalla.
//
// Se llama SIEMPRE DESPUÉS de que la acción ocurrió. Nunca antes, y nunca dentro del `await` que la
// hace: primero se cobra, después se mide.
export function medirAccion(pantalla: string, accion: string): void {
  encolar({ pantalla, accion });
}

// medirToque cuenta que el dedo cayó en una zona de una pantalla.
//
// Recibe la CELDA, no el punto: quien la calcula es `celdaDelToque`, en la tableta. Si aquí entrara
// un (x, y), el dato fino existiría en el cuerpo del request y en el log de cualquier proxy aunque
// el servidor lo redondeara después.
export function medirToque(pantalla: string, celda: number, orientacion: 'horizontal' | 'vertical'): void {
  encolar({ pantalla, celda, orientacion });
}

function esToque(x: EventoDeUso | ToqueDeUso): x is ToqueDeUso {
  return 'celda' in x;
}

function encolar(e: EventoDeUso | ToqueDeUso): void {
  cola.push(e);
  if (cola.length > MAX_EN_COLA) {
    cola = cola.slice(cola.length - MAX_EN_COLA);
  }
  arrancarReloj();
  if (cola.length >= TOPE_DEL_LOTE) vaciarCola();
}

function arrancarReloj(): void {
  if (reloj !== null) return;
  reloj = setInterval(vaciarCola, CADA_MS);
}

// vaciarCola manda lo que haya. Exportada porque el cierre de la pestaña la llama directo.
export function vaciarCola(): void {
  // UN SOLO LOTE EN VUELO A LA VEZ, y esta guarda es la razón de ser de la función.
  //
  // El caso malo no es la red caída —ésa falla rápido— sino la LENTA: con envíos de 8 a 15
  // segundos, el reloj de 10 dispara el siguiente encima, y a la media hora hay varios lotes
  // compitiendo por la misma conexión flaky con el POST del cobro. No bloquean al operador, pero le
  // hacen más lento lo único que importa.
  if (enVuelo || cola.length === 0) return;

  const token = useSessionStore.getState().token;
  // Sin sesión el servidor no sabría de qué empresa ni de qué rol es —los pone él, no el cliente—,
  // así que mandarlo sería gastar la red del mostrador para recibir un 401.
  if (!token) return;

  const lote = cola;
  cola = [];
  enVuelo = true;
  const eventos = lote.filter((x) => !esToque(x));
  const toques = lote.filter(esToque);

  // SIN `await`, a propósito. Quien llamó a medir ya siguió con lo suyo hace rato.
  void fetch(BASE + '/usage', {
    method: 'POST',
    // `keepalive` termina el envío aunque la pestaña se cierre. Es lo único que `sendBeacon` daba
    // de más, y aquí hace falta el header de autenticación, que el beacon no admite.
    keepalive: true,
    headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
    body: JSON.stringify({ eventos, toques }),
  })
    .catch(() => {
      // Se perdió. No se reintenta, no se avisa y no se vuelve a encolar: un reintento es otra
      // petición compitiendo con el cobro por la misma red mala.
    })
    .finally(() => {
      enVuelo = false;
    });
}

// medirRecargaONavegacion decide si la primera pantalla de esta carga cuenta.
//
// El operador aprieta F5 por costumbre y eso no es «volvió a entrar». Se le pregunta al navegador
// qué tipo de navegación fue, en vez de adivinarlo con un cronómetro: una ventana de N segundos se
// equivoca por los dos lados —un F5 con caché frío tarda más que la ventana, y un cajero que rebota
// entre dos pantallas en segundos cuenta una sola vez—.
export function esRecargaDeLaPagina(): boolean {
  try {
    const nav = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined;
    return nav?.type === 'reload';
  } catch {
    // Un navegador sin esta API cuenta la recarga como una apertura más. Es el error barato.
    return false;
  }
}

// Al irse la pestaña se manda lo que quede: es el último momento en que se puede.
//
// DOS EVENTOS Y NO UNO, y esto lo encontró el e2e: `visibilitychange` cubre cambiar de pestaña o
// minimizar, pero NO se dispara de forma confiable cuando la página NAVEGA a otra dirección —que es
// lo que pasa al recargar o al entrar por una URL—. Con solo ése, todo lo encolado en una carga se
// perdía al salir de ella y el registrador parecía apagado: cero peticiones en una sesión entera.
// `pagehide` es el que cubre ese caso.
if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') vaciarCola();
  });
  window.addEventListener('pagehide', () => vaciarCola());
}

// AL CAMBIAR DE SESIÓN, LA COLA SE TIRA. No se manda: se tira.
//
// El caso concreto: un cajero navega tres pantallas —tres eventos encolados, por debajo del lote de
// 20 y antes de los 10 segundos—, se desloguea, y entra otro operador en la misma tableta. Como el
// token se lee al VACIAR y no al encolar, esos tres eventos viajarían con el Bearer del siguiente y
// el servidor los contaría con SU empresa y SU rol, porque los saca del token. La medición
// quedaría atribuida a quien no fue.
//
// Se descartan en vez de mandarlos antes de cerrar sesión porque esta feature tiene permiso para
// perder mediciones y no lo tiene para mentir. Es la misma razón por la que el carrito SÍ se
// conserva entre sesiones de la misma empresa y esto no: un ticket a medias vale; tres aperturas
// de pantalla, no.
useSessionStore.subscribe((estado, anterior) => {
  if (estado.token !== anterior.token) {
    cola = [];
  }
});

// _soloParaPruebas expone lo mínimo para poder probar la cola sin abrir la caja negra.
export const _soloParaPruebas = {
  reiniciar(): void {
    cola = [];
    enVuelo = false;
    if (reloj !== null) {
      clearInterval(reloj);
      reloj = null;
    }
  },
  tamanoDeLaCola(): number {
    return cola.length;
  },
};
