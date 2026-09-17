import { describe, expect, it } from 'vitest';
import { minutosQueQuedan, urgenciaDe } from './urgencia';

const enMinutos = (m: number, ahora: number) => new Date(ahora + m * 60000).toISOString();

describe('urgencia de un pedido de plataforma', () => {
  const ahora = Date.UTC(2026, 8, 17, 18, 0, 0);

  // LOS DOS MOMENTOS QUE CAMBIAN EL COMPORTAMIENTO, y los únicos que la pantalla tiene que
  // distinguir: cuándo empieza a sonar el teléfono del local y cuándo está por expirar.
  it('recién llegado es normal', () => {
    expect(urgenciaDe(enMinutos(11, ahora), ahora)).toBe('normal');
  });

  it('pasados 90 segundos, el teléfono del local ya está sonando', () => {
    // Plazo 11.5 min; a los 90 s quedan 10 min.
    expect(urgenciaDe(enMinutos(10, ahora), ahora)).toBe('sonando');
  });

  it('cerca del final está por expirar', () => {
    expect(urgenciaDe(enMinutos(1, ahora), ahora)).toBe('por_expirar');
  });

  it('un plazo ya vencido sigue siendo por expirar y no se vuelve normal', () => {
    expect(urgenciaDe(enMinutos(-5, ahora), ahora)).toBe('por_expirar');
  });

  it('sin plazo no inventa urgencia', () => {
    expect(urgenciaDe(null, ahora)).toBe('normal');
    expect(urgenciaDe('no es una fecha', ahora)).toBe('normal');
  });
});

describe('minutos que quedan', () => {
  const ahora = Date.UTC(2026, 8, 17, 18, 0, 0);

  // REDONDEA HACIA ABAJO. Decir «3 minutos» cuando quedan 3:50 promete tiempo que no hay, y eso
  // cuesta un pedido cancelado. Hacia abajo solo puede apurar de más.
  it('3 minutos y 50 segundos son 3 minutos, no 4', () => {
    const falta = new Date(ahora + 3 * 60000 + 50000).toISOString();
    expect(minutosQueQuedan(falta, ahora)).toBe(3);
  });

  it('un plazo vencido son cero minutos y nunca un negativo', () => {
    expect(minutosQueQuedan(enMinutos(-2, ahora), ahora)).toBe(0);
  });

  it('sin plazo son cero', () => {
    expect(minutosQueQuedan(null, ahora)).toBe(0);
  });
});
