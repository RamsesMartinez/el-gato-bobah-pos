// La alarma de pedido entrante (spec 093): un sonido continuo mientras haya algo por aceptar.
//
// UN BÚFER EN BUCLE Y NO UN TEMPORIZADOR. El navegador frena `setInterval` en cuanto la pestaña
// pierde el primer plano; un `AudioBufferSourceNode` con `loop` lo reproduce el motor de audio y
// sigue sonando igual. El tono se genera en memoria: no hay archivo que cargar ni que se pierda de
// la caché.
//
// EL NAVEGADOR NO DEJA SONAR HASTA EL PRIMER TOQUE. La alarma no finge que suena: dice «bloqueada»,
// y la franja pinta el botón para activarla. Cualquier toque en la página la desbloquea.

export type EstadoAlarma = 'callada' | 'sonando' | 'bloqueada';

/** Lo que la alarma usa de un AudioContext; el resto del API no le importa. */
export interface ContextoDeAudio {
  state: AudioContextState;
  sampleRate: number;
  destination: AudioNode | object;
  resume(): Promise<void>;
  createBuffer(canales: number, largo: number, frecuencia: number): AudioBuffer;
  createBufferSource(): AudioBufferSourceNode;
  addEventListener(tipo: 'statechange', fn: () => void): void;
}

/** Genera el sonido que se repite. Cambiar el tono es cambiar esta función (FR-013). */
export type Tono = (ctx: ContextoDeAudio) => AudioBuffer;

// Dos pitidos agudos y una pausa, en un ciclo de 2 s: se oye sobre el ruido de una cocina sin ser
// un timbre continuo que nadie aguanta.
export const tonoPorOmision: Tono = (ctx) => {
  const largo = Math.floor(ctx.sampleRate * 2);
  const buf = ctx.createBuffer(1, largo, ctx.sampleRate);
  const datos = buf.getChannelData(0);
  const pitido = (inicio: number, dur: number) => {
    const a = Math.floor(inicio * ctx.sampleRate);
    const n = Math.floor(dur * ctx.sampleRate);
    for (let i = 0; i < n && a + i < largo; i++) {
      // Rampa de 5 ms al entrar y al salir: sin ella cada pitido suena con un chasquido.
      const rampa = Math.min(1, i / (0.005 * ctx.sampleRate), (n - i) / (0.005 * ctx.sampleRate));
      datos[a + i] = 0.6 * rampa * Math.sin((2 * Math.PI * 880 * i) / ctx.sampleRate);
    }
  };
  pitido(0, 0.18);
  pitido(0.3, 0.18);
  return buf;
};

function contextoDelNavegador(): ContextoDeAudio | null {
  // En iOS, Web Audio obedece el interruptor de silencio: la alarma diría «sonando» sin que se oiga.
  // Como reproducción (Safari 16.4+) suena aunque el iPad esté en silencio.
  const sesion = (navigator as unknown as { audioSession?: { type: string } }).audioSession;
  if (sesion) sesion.type = 'playback';
  const Ctor = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  return Ctor ? (new Ctor() as unknown as ContextoDeAudio) : null;
}

export interface Alarma {
  /** Pide que suene (hay pendientes) o que calle (no hay). Idempotente: no apila sonidos. */
  sonar(debe: boolean): void;
  /** Intenta destrabar el audio. Se llama en el primer toque y al volver la tableta. */
  desbloquear(): void;
  estado(): EstadoAlarma;
  suscribir(fn: () => void): () => void;
}

export function crearAlarma(opts: { contexto?: () => ContextoDeAudio | null; tono?: Tono } = {}): Alarma {
  const crear = opts.contexto ?? contextoDelNavegador;
  const tono = opts.tono ?? tonoPorOmision;
  let ctx: ContextoDeAudio | null | undefined;
  let fuente: AudioBufferSourceNode | null = null;
  let debe = false;
  let estado: EstadoAlarma = 'callada';
  const oyentes = new Set<() => void>();

  const contexto = () => {
    if (ctx === undefined) {
      try {
        ctx = crear();
      } catch {
        ctx = null;
      }
      ctx?.addEventListener('statechange', aplicar);
    }
    return ctx;
  };

  function descartar() {
    try {
      fuente?.stop();
    } catch {
      /* ya estaba detenida */
    }
    fuente = null;
    ctx = undefined;
  }

  function fijar(nuevo: EstadoAlarma) {
    if (nuevo === estado) return;
    estado = nuevo;
    oyentes.forEach((f) => f());
  }

  function aplicar() {
    if (!debe) {
      try {
        fuente?.stop();
      } catch {
        /* ya estaba detenida */
      }
      fuente = null;
      fijar('callada');
      return;
    }
    const c = contexto();
    if (!c || c.state !== 'running') {
      fijar('bloqueada');
      return;
    }
    if (!fuente) {
      fuente = c.createBufferSource();
      fuente.buffer = tono(c);
      fuente.loop = true;
      fuente.connect(c.destination as AudioNode);
      fuente.start();
    }
    fijar('sonando');
  }

  return {
    sonar(d) {
      debe = d;
      aplicar();
      // Si el navegador ya lo permitía (un toque anterior en esta carga), esto lo arranca.
      if (d && estado === 'bloqueada') this.desbloquear();
    },
    desbloquear() {
      // Un contexto cerrado no revive: se tira y se crea otro (iOS lo cierra tras una llamada).
      if (ctx && (ctx.state as string) === 'closed') descartar();
      const c = contexto();
      if (!c || c.state === 'running') {
        aplicar();
        return;
      }
      c.resume().then(aplicar, () => {
        // `interrupted` en iOS: resume() rechaza para siempre. Uno nuevo sí arranca. Solo se tira el
        // que falló: con dos intentos encimados, el segundo tiraría el bueno y quedarían dos bucles.
        if (ctx === c) descartar();
        const nuevo = contexto();
        if (!nuevo) {
          aplicar();
          return;
        }
        nuevo.resume().then(aplicar, aplicar);
      });
    },
    estado: () => estado,
    suscribir(fn) {
      oyentes.add(fn);
      return () => oyentes.delete(fn);
    },
  };
}

/** La alarma de la aplicación. El AudioContext se crea en su primer uso, no al importar. */
export const alarmaDeLaApp = crearAlarma();
