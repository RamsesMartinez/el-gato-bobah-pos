import { describe, expect, it } from 'vitest';
import { vencio, proximoVencimiento, marcarBloqueada, estabaBloqueada, limpiarBloqueo } from './inactividad';

describe('cuándo se bloquea la pantalla', () => {
  it('no vence antes del tiempo configurado', () => {
    expect(vencio(1_000, 1_000 + 179_000, 180)).toBe(false);
  });

  it('vence al cumplirse el tiempo', () => {
    expect(vencio(1_000, 1_000 + 180_000, 180)).toBe(true);
  });

  // Cero es una elección válida —una caja en una oficina cerrada no necesita bloquearse— y
  // tratarlo como "vence de inmediato" dejaría la tableta inservible para quien lo eligió.
  it('con 0 no se bloquea nunca', () => {
    expect(vencio(1_000, 1_000 + 999_999_000, 0)).toBe(false);
  });

  // Un valor negativo solo puede venir de datos corruptos. Se trata como 0 en vez de bloquear a
  // cada instante: el modo de fallo tiene que dejar trabajar, no impedirlo.
  it('un valor negativo tampoco bloquea', () => {
    expect(vencio(1_000, 1_000 + 999_999_000, -5)).toBe(false);
  });

  it('el próximo vencimiento sale de la última actividad', () => {
    expect(proximoVencimiento(5_000, 180)).toBe(5_000 + 180_000);
  });

  it('sin bloqueo configurado no hay próximo vencimiento', () => {
    expect(proximoVencimiento(5_000, 0)).toBeNull();
  });
});

// Un sessionStorage de mentiras para los tests.
function enMemoria() {
  const m = new Map<string, string>();
  return {
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => { m.set(k, v); },
    removeItem: (k: string) => { m.delete(k); },
  };
}

describe('el bloqueo sobrevive a una recarga', () => {
  // EL ATAQUE: la tableta queda sola y bloqueada. Basta F5 —o el pull-to-refresh de la PWA— para
  // que React arranque de cero con `bloqueado = false`, y restoreSession canjee la cookie viva por
  // una sesión completa del operador anterior. Sin PIN. Todo lo que se cobre queda a su nombre.
  //
  // Por eso la marca de bloqueo vive FUERA de React.
  it('recuerda que estaba bloqueada', () => {
    const almacen = enMemoria();
    marcarBloqueada(almacen);
    expect(estabaBloqueada(almacen)).toBe(true);
  });

  it('sin marca previa, no arranca bloqueada', () => {
    expect(estabaBloqueada(enMemoria())).toBe(false);
  });

  it('desbloquear borra la marca', () => {
    const almacen = enMemoria();
    marcarBloqueada(almacen);
    limpiarBloqueo(almacen);
    expect(estabaBloqueada(almacen)).toBe(false);
  });

  // El almacenamiento puede fallar o estar deshabilitado (ventana privada, políticas del navegador).
  // El modo de fallo tiene que ser BLOQUEAR, no dejar pasar: es una protección, y una protección
  // que se cae sola ante un error del navegador no protege de nada.
  it('si el almacén truena al leer, se asume bloqueada', () => {
    const roto = { getItem() { throw new Error('sin acceso'); }, setItem() {}, removeItem() {} };
    expect(estabaBloqueada(roto)).toBe(true);
  });

  it('si el almacén truena al escribir, no revienta la aplicación', () => {
    const roto = { getItem: () => null, setItem() { throw new Error('lleno'); }, removeItem() {} };
    expect(() => marcarBloqueada(roto)).not.toThrow();
  });
});
