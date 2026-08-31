import { describe, expect, it } from 'vitest';
import { ANIMALES, nombreLibre } from './folio';

describe('nombreLibre', () => {
  it('no repite lo que ya usan las cuentas abiertas', () => {
    // El primero de la lista está tomado, así que con un azar que siempre elige la posición 0
    // tiene que salir el SEGUNDO, no el tomado.
    const tomado = ANIMALES[0];
    expect(nombreLibre([tomado], () => 0)).toBe(ANIMALES[1]);
  });

  it('sin cuentas abiertas puede elegir cualquiera', () => {
    expect(nombreLibre([], () => 0)).toBe(ANIMALES[0]);
  });

  // Con las 100 cuentas abiertas a la vez —que no pasa— devuelve uno repetido en vez de vacío:
  // el servidor lo desempata agregándole la vuelta.
  it('con todo tomado devuelve un animal igual', () => {
    const got = nombreLibre([...ANIMALES], () => 0);
    expect(ANIMALES).toContain(got);
  });

  it('siempre devuelve un animal de la lista', () => {
    for (let i = 0; i < 200; i++) {
      expect(ANIMALES).toContain(nombreLibre([]));
    }
  });
});
