import { describe, it, expect } from 'vitest';
import { hayQuePedirElFolio } from './folioPlataforma';

describe('cuándo se interpone la hoja del folio', () => {
  it('no se interpone en mostrador, aunque quedara texto viejo', () => {
    // El campo no existe sin plataforma; si quedara algo escrito, pedirlo sería un toque regalado.
    expect(hayQuePedirElFolio(null, '')).toBe(false);
    expect(hayQuePedirElFolio(null, 'UBER-1')).toBe(false);
  });

  it('se interpone con plataforma activa y el campo vacío', () => {
    expect(hayQuePedirElFolio(6, '')).toBe(true);
  });

  it('se interpone si el campo tiene solo espacios', () => {
    // "Lo toqué y no escribí". El servidor lo rechazaría igual, pero descubrirlo hasta el rechazo
    // cuesta el viaje completo con el repartidor enfrente.
    expect(hayQuePedirElFolio(6, '   ')).toBe(true);
  });

  it('NO se interpone con el campo lleno: mandar cuesta los mismos toques que hoy', () => {
    // Es la mitad de SC-003: el toque extra solo lo paga quien no capturó el folio.
    expect(hayQuePedirElFolio(6, '4B2E9A10')).toBe(false);
    expect(hayQuePedirElFolio(6, '  4B2E9A10  ')).toBe(false);
  });
});

// Una cuenta guardada por una versión anterior no trae el campo. La regla no puede reventar con
// una entrada que el sistema sí produce — ese `.trim()` sobre undefined dejó el POS en blanco.
it('no truena si la cuenta viene sin el campo', () => {
  expect(() => hayQuePedirElFolio(6, undefined)).not.toThrow();
  expect(hayQuePedirElFolio(6, undefined)).toBe(true);
  expect(hayQuePedirElFolio(null, undefined)).toBe(false);
});
