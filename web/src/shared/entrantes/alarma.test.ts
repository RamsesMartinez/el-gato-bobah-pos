import { describe, expect, it, vi } from 'vitest';
import { crearAlarma, type ContextoDeAudio } from './alarma';

// Un AudioContext falso: el navegador real no deja sonar sin un toque, y eso es justo lo que se
// prueba. `permitido` decide si `resume()` lo deja corriendo.
function contextoFalso(permitido: boolean) {
  const fuentes: { start: ReturnType<typeof vi.fn>; stop: ReturnType<typeof vi.fn>; loop: boolean; buffer: unknown }[] = [];
  const oyentes: (() => void)[] = [];
  const ctx = {
    state: 'suspended' as AudioContextState,
    permitido,
    sampleRate: 8000,
    destination: {},
    resume: vi.fn(async () => {
      if (ctx.permitido) {
        ctx.state = 'running';
        oyentes.forEach((f) => f());
      }
    }),
    createBuffer: vi.fn((_c: number, n: number) => ({ getChannelData: () => new Float32Array(n), length: n })),
    createBufferSource: vi.fn(() => {
      const f = { start: vi.fn(), stop: vi.fn(), loop: false, buffer: null as unknown, connect: vi.fn() };
      fuentes.push(f);
      return f;
    }),
    addEventListener: (_: string, f: () => void) => oyentes.push(f),
  };
  return { ctx: ctx as unknown as ContextoDeAudio & { permitido: boolean; state: AudioContextState }, fuentes };
}

const espera = () => new Promise((r) => setTimeout(r, 0));

describe('alarma', () => {
  it('con un pendiente y el sonido permitido, suena en bucle', async () => {
    const { ctx, fuentes } = contextoFalso(true);
    const a = crearAlarma({ contexto: () => ctx });
    a.desbloquear();
    await espera();
    a.sonar(true);
    expect(a.estado()).toBe('sonando');
    expect(fuentes).toHaveLength(1);
    expect(fuentes[0].loop).toBe(true);
    expect(fuentes[0].start).toHaveBeenCalled();
  });

  // FR-011: con varios pendientes, UNA alarma. Pedir que suene otra vez no apila otro bucle.
  it('pedirle que suene dos veces no superpone dos sonidos', async () => {
    const { ctx, fuentes } = contextoFalso(true);
    const a = crearAlarma({ contexto: () => ctx });
    a.desbloquear();
    await espera();
    a.sonar(true);
    a.sonar(true);
    expect(fuentes).toHaveLength(1);
  });

  it('sin pendientes se calla', async () => {
    const { ctx, fuentes } = contextoFalso(true);
    const a = crearAlarma({ contexto: () => ctx });
    a.desbloquear();
    await espera();
    a.sonar(true);
    a.sonar(false);
    expect(fuentes[0].stop).toHaveBeenCalled();
    expect(a.estado()).toBe('callada');
  });

  // EL BORDE QUE IMPORTA: el navegador no deja sonar hasta el primer toque. La alarma no finge que
  // suena: queda «bloqueada», que es lo que hace aparecer el botón para activarla.
  it('con el navegador bloqueando el sonido queda bloqueada, y el primer toque la hace sonar', async () => {
    const { ctx, fuentes } = contextoFalso(false);
    const a = crearAlarma({ contexto: () => ctx });
    const cambios = vi.fn();
    a.suscribir(cambios);
    a.sonar(true);
    await espera();
    expect(a.estado()).toBe('bloqueada');
    expect(fuentes).toHaveLength(0);

    ctx.permitido = true; // la persona tocó la pantalla
    a.desbloquear();
    await espera();
    expect(ctx.resume).toHaveBeenCalled();
    expect(a.estado()).toBe('sonando');
    expect(fuentes).toHaveLength(1);
    expect(cambios).toHaveBeenCalled();
  });

  it('si después del toque sigue bloqueada, lo dice', async () => {
    const { ctx } = contextoFalso(false);
    const a = crearAlarma({ contexto: () => ctx });
    a.sonar(true);
    a.desbloquear();
    await espera();
    expect(a.estado()).toBe('bloqueada');
  });

  // FR-013: el tono es una pieza aparte. Cambiarlo no toca la lógica de cuándo suena.
  it('usa el tono que se le pase', async () => {
    const { ctx, fuentes } = contextoFalso(true);
    const miTono = vi.fn(() => ({ propio: true }) as unknown as AudioBuffer);
    const a = crearAlarma({ contexto: () => ctx, tono: miTono });
    a.desbloquear();
    await espera();
    a.sonar(true);
    expect(miTono).toHaveBeenCalledWith(ctx);
    expect(fuentes[0].buffer).toEqual({ propio: true });
  });

  it('sin AudioContext en el navegador no truena: queda bloqueada', () => {
    const a = crearAlarma({ contexto: () => null });
    a.sonar(true);
    expect(a.estado()).toBe('bloqueada');
  });
});
