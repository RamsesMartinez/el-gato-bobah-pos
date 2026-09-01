import { describe, expect, it } from 'vitest';
import { vencio, proximoVencimiento } from './inactividad';

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
