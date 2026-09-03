import { describe, expect, it } from 'vitest';
import { nombreLibre } from './folio';

const LISTA = ['Tigre', 'Zorro', 'Búho'];

describe('nombreLibre', () => {
  it('no repite lo que ya usan las cuentas abiertas', () => {
    // Con un azar que siempre elige la posición 0, si el primero está tomado sale el segundo.
    expect(nombreLibre(LISTA, ['Tigre'], () => 0)).toBe('Zorro');
  });

  it('sin cuentas abiertas puede elegir cualquiera', () => {
    expect(nombreLibre(LISTA, [], () => 0)).toBe('Tigre');
  });

  // Haría falta una cuenta abierta por cada animal. Devuelve uno repetido en vez de vacío: el
  // servidor lo desempata agregándole la vuelta.
  it('con todo tomado devuelve un animal de la lista', () => {
    expect(LISTA).toContain(nombreLibre(LISTA, [...LISTA], () => 0));
  });

  // La lista llega por red. Mientras no esté, la cuenta se queda sin nombre y el servidor le pone
  // el suyo al cobrar; lo que no puede es inventar uno que después cambie en el ticket.
  it('sin lista todavía, no inventa nombre', () => {
    expect(nombreLibre([], [])).toBe('');
  });

  it('siempre devuelve un animal de la lista', () => {
    for (let i = 0; i < 100; i++) expect(LISTA).toContain(nombreLibre(LISTA, []));
  });
});
