import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useSessionStore } from '../stores/session';
import { medirAccion, medirPantalla, vaciarCola, _soloParaPruebas } from './uso';

// EL REGISTRADOR DE USO NO PUEDE ESTORBAR (US3, FR-004).
//
// Todo lo de aquí prueba lo mismo desde ángulos distintos: que medir nunca se meta en el camino del
// operador. Es la condición para que esta feature sea aceptable, no una cualidad deseable.

function respuestaOk() {
  return Promise.resolve({ ok: true, status: 204 } as Response);
}

// El stub de `fetch` se tipa como el de verdad para poder leer lo que se mandó sin castear a ciegas.
type LlamadaAFetch = [input: string, init: RequestInit];

beforeEach(() => {
  vi.useFakeTimers();
  _soloParaPruebas.reiniciar();
  useSessionStore.setState({ token: 'tok-de-prueba' });
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('el registrador de uso', () => {
  it('junta los eventos y manda UNO solo al llegar al tope', () => {
    const fetchStub = vi.fn(respuestaOk);
    vi.stubGlobal('fetch', fetchStub);

    for (let i = 0; i < 19; i++) medirPantalla('pos');
    expect(fetchStub).not.toHaveBeenCalled(); // 19 todavía no

    medirPantalla('pos'); // el veinte
    expect(fetchStub).toHaveBeenCalledTimes(1);

    const [, init] = fetchStub.mock.calls[0] as unknown as LlamadaAFetch;
    const cuerpo = JSON.parse(init.body as string);
    expect(cuerpo.eventos).toHaveLength(20);
    // `keepalive` es lo que hace que el envío termine aunque la pestaña se cierre.
    expect(init.keepalive).toBe(true);
  });

  it('manda lo que haya cada 10 segundos', () => {
    const fetchStub = vi.fn(respuestaOk);
    vi.stubGlobal('fetch', fetchStub);

    medirPantalla('caja');
    expect(fetchStub).not.toHaveBeenCalled();

    vi.advanceTimersByTime(10_000);
    expect(fetchStub).toHaveBeenCalledTimes(1);
  });

  it('NUNCA hay dos envíos en vuelo a la vez', async () => {
    // El caso que este test cubre no es la red caída —ésa falla rápido— sino la LENTA: un envío
    // que tarda 15 segundos mientras el temporizador de 10 dispara el siguiente. Sin la guarda, los
    // lotes se apilan y compiten por la conexión con el cobro.
    let resolver: (r: Response) => void = () => {};
    const pendiente = new Promise<Response>((r) => {
      resolver = r;
    });
    const fetchStub = vi.fn(() => pendiente);
    vi.stubGlobal('fetch', fetchStub);

    medirPantalla('pos');
    vi.advanceTimersByTime(10_000); // sale el primero y se queda colgado
    expect(fetchStub).toHaveBeenCalledTimes(1);

    medirPantalla('pos');
    vi.advanceTimersByTime(10_000); // el segundo NO puede salir encima
    expect(fetchStub).toHaveBeenCalledTimes(1);

    resolver({ ok: true, status: 204 } as Response);
    await vi.runOnlyPendingTimersAsync();
    vi.advanceTimersByTime(10_000);
    expect(fetchStub).toHaveBeenCalledTimes(2); // ahora sí, con lo acumulado
  });

  it('un fetch que revienta no lanza al que midió, y no reintenta', async () => {
    const fetchStub = vi.fn(() => Promise.reject(new TypeError('Failed to fetch')));
    vi.stubGlobal('fetch', fetchStub);

    expect(() => {
      medirPantalla('pos');
      vaciarCola();
    }).not.toThrow();

    await vi.runOnlyPendingTimersAsync();
    vi.advanceTimersByTime(60_000);
    // Lo que se perdió, se perdió: una medición vale menos que un reintento compitiendo con el cobro.
    expect(fetchStub).toHaveBeenCalledTimes(1);
  });

  it('la cola tiene tope: lo viejo se tira antes que crecer sin fin', () => {
    const fetchStub = vi.fn(() => new Promise<Response>(() => {})); // nunca resuelve
    vi.stubGlobal('fetch', fetchStub);

    // Con el primer envío colgado, todo lo demás se acumula.
    for (let i = 0; i < 200; i++) medirPantalla('pos');
    expect(_soloParaPruebas.tamanoDeLaCola()).toBeLessThanOrEqual(50);
  });

  it('sin sesión no se manda nada', () => {
    const fetchStub = vi.fn(respuestaOk);
    vi.stubGlobal('fetch', fetchStub);
    useSessionStore.setState({ token: null });

    medirAccion('pos', 'cobrar');
    vaciarCola();
    // Sin token el servidor no sabría de qué empresa es, y responde 401. Mandarlo sería gastar red
    // del mostrador para nada.
    expect(fetchStub).not.toHaveBeenCalled();
  });
});

describe('la cola y el cambio de sesión', () => {
  it('cambiar de sesión TIRA lo encolado, no lo manda con el token nuevo', () => {
    const fetchStub = vi.fn(respuestaOk);
    vi.stubGlobal('fetch', fetchStub);

    // Un cajero deja tres eventos encolados y se va.
    medirPantalla('pos');
    medirPantalla('caja');
    medirPantalla('pedidos');
    expect(_soloParaPruebas.tamanoDeLaCola()).toBe(3);

    // Entra otro operador en la misma tableta.
    useSessionStore.setState({ token: 'tok-de-otro' });
    expect(_soloParaPruebas.tamanoDeLaCola()).toBe(0);

    vaciarCola();
    // Si se hubieran mandado, el servidor los habría contado con la empresa y el rol del SEGUNDO:
    // saca las dos cosas del token, no del cuerpo.
    expect(fetchStub).not.toHaveBeenCalled();
  });

  it('cerrar sesión también la tira', () => {
    const fetchStub = vi.fn(respuestaOk);
    vi.stubGlobal('fetch', fetchStub);
    medirPantalla('pos');
    useSessionStore.setState({ token: null });
    expect(_soloParaPruebas.tamanoDeLaCola()).toBe(0);
  });
});
