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

// Los rechazos de la cuenta en captura (spec 030) se dicen para quien opera: nunca «borrador»,
// «versión» ni el código del servidor.
describe('rechazos de la cuenta', () => {
  test.each([
    ['DRAFT_CHANGED', /otra tableta/i],
    ['DRAFT_DISCARDED', /descart/i],
    ['DRAFT_SENT', /cocina/i],
    ['ORDER_CLOSED', /cuenta nueva/i],
    ['PLATFORM_ORDER_NO_LINES', /plataforma/i],
  ])('%s', (code, esperado) => {
    const m = mensajeDeError(new ApiError(409, code, 'draft version mismatch', 'r'));
    expect(m).toMatch(esperado);
    expect(m).not.toMatch(/borrador|draft|versi[oó]n|409|DRAFT|ORDER/i);
  });
});

// La caja con asistencia (spec 046): frases para quien opera, y la de quien abrió conserva el nombre.
describe('rechazos de la caja por asistencia', () => {
  test.each([
    ['CLOCK_IN_REQUIRED', /marca tu entrada/i],
    ['CLOCK_OUT_WITH_PIN', /tu PIN/],
  ])('%s', (code, esperado) => {
    const m = mensajeDeError(new ApiError(409, code, 'x', 'r'));
    expect(m).toMatch(esperado);
    expect(m).not.toMatch(/409|CLOCK/);
  });

  test('OPENER_NOT_CLOCKED_OUT dice quién abrió', () => {
    expect(mensajeDeError(new ApiError(409, 'OPENER_NOT_CLOCKED_OUT', 'Ana abrió esta caja y no ha marcado su salida', 'r')))
      .toBe('Ana abrió esta caja y no ha marcado su salida');
  });
});

test('ATTENDANCE_STATE_CHANGED pide volver a intentar, sin el texto en minúsculas del servidor', () => {
  expect(mensajeDeError(new ApiError(409, 'ATTENDANCE_STATE_CHANGED', 'el registro de esta persona cambió, vuelve a intentarlo', 'r')))
    .toBe('La asistencia cambió en otra tableta. Vuelve a intentarlo.');
});
