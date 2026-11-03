import { describe, expect, test } from 'vitest';
import { ApiError } from '../../api/client';
import { clockError } from './clockErrors';

describe('clockError', () => {
  test('el bloqueo por PIN dice en cuántos minutos, redondeado hacia arriba', () => {
    expect(clockError(new ApiError(429, 'TOO_MANY_REQUESTS', 'x', 'r', undefined, 61))).toEqual(
      { kind: 'too_many', message: 'Demasiados intentos. Intenta de nuevo en 2 min.' });
    expect(clockError(new ApiError(429, 'TOO_MANY_REQUESTS', 'x', 'r')).message).toContain('1 min');
  });

  test('otra tableta ya checó a esa persona', () => {
    expect(clockError(new ApiError(409, 'ATTENDANCE_STATE_CHANGED', 'x', 'r')).kind).toBe('state_changed');
  });

  test('sin red no culpa al PIN', () => {
    expect(clockError(new ApiError(0, 'NETWORK', 'x', 'r'))).toEqual(
      { kind: 'offline', message: 'Sin conexión. Checa cuando vuelva la red.' });
  });

  // Persona inexistente, sin PIN o PIN equivocado se dicen igual: distinguirlos enumera usuarios.
  test('cualquier otro rechazo es «No coincide»', () => {
    for (const e of [new ApiError(401, 'INVALID_CREDENTIALS', 'x', 'r'), new Error('x')]) {
      expect(clockError(e)).toEqual({ kind: 'no_match', message: 'No coincide. Inténtalo de nuevo.' });
    }
  });
});
