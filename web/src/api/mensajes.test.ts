import { describe, expect, test } from 'vitest';
import { ApiError } from './client';
import { mensajeDeError } from './mensajes';

describe('mensajeDeError', () => {
  test('un rechazo del servidor se muestra tal cual', () => {
    expect(mensajeDeError(new ApiError(409, 'CONFLICT', 'No puedes cobrar más de lo que falta', 'req-1')))
      .toBe('No puedes cobrar más de lo que falta');
  });

  // "TypeError: Failed to fetch" es lo que salía en pantalla al caerse la red. No le dice nada a
  // quien opera y le dice todo a quien programó.
  test('una caída de red se traduce a algo accionable', () => {
    const m = mensajeDeError(new TypeError('Failed to fetch'));
    expect(m).not.toMatch(/TypeError|fetch/i);
    expect(m).toMatch(/conexión/i);
  });

  test('cualquier otra cosa no filtra el objeto', () => {
    const m = mensajeDeError({ stack: 'at Object.<anonymous> (/app/src/x.ts:1:1)' });
    expect(m).not.toMatch(/stack|anonymous|\.ts/);
    expect(m.length).toBeGreaterThan(10);
  });
});
