import { describe, expect, test } from 'vitest';
import {
  avisoDeInventario, montoDevolvible, porQueNoSeDevuelve, sePuedeDevolver,
} from './devolucion';

describe('montoDevolvible', () => {
  test.each([
    [500, 0, 500],
    [500, 440, 60],
    [500, 500, 0],
    [0, 0, 0],
    // Nunca negativo: si se devolvió de más, lo que queda es cero, no una deuda del cliente.
    [500, 600, 0],
  ])('cobrado %s, devuelto %s -> %s', (cobrado, devuelto, quiere) => {
    expect(montoDevolvible(cobrado, devuelto)).toBe(quiere);
  });
});

describe('sePuedeDevolver', () => {
  // EL TABLERO OFRECÍA "REEMBOLSAR" JUNTO A "COBRAR $220" EN LA MISMA TARJETA.
  //
  // Tocarlo anotaba $220 de pérdida por un ingreso que nunca ocurrió, y la cuenta por cobrar
  // desaparecía del contador sin haberse cobrado.
  test('un pedido sin cobros no se devuelve', () => {
    expect(sePuedeDevolver(220, 0, 0, 'se equivocó')).toBe('sin-cobros');
  });

  test('lo ya devuelto no se devuelve otra vez', () => {
    expect(sePuedeDevolver(1, 500, 500, 'otra vez')).toBe('ya-devuelto');
  });

  test('no se devuelve más de lo que entró', () => {
    expect(sePuedeDevolver(500.01, 500, 0, 'de más')).toBe('excede');
    expect(sePuedeDevolver(61, 500, 440, 'de más')).toBe('excede');
  });

  test.each([0, -50, NaN, Infinity])('un monto absurdo (%s) se rechaza', (m) => {
    expect(sePuedeDevolver(m, 500, 0, 'motivo')).toBe('sin-monto');
  });

  // El motivo es lo que deja explicar la devolución en el corte. Un espacio no es un motivo: pasaba
  // los dos lados y llegaba a la base, donde el check lo daba por bueno.
  test.each(['', ' ', '\t'])('un motivo en blanco (%s) se rechaza', (m) => {
    expect(sePuedeDevolver(100, 500, 0, m)).toBe('sin-motivo');
  });

  test('lo que sí se puede, pasa', () => {
    expect(sePuedeDevolver(500, 500, 0, 'se equivocó el platillo')).toBeNull();
    expect(sePuedeDevolver(60, 500, 440, 'el resto')).toBeNull();
  });
});

// El texto es para quien atiende el negocio, no para quien lo programó.
describe('porQueNoSeDevuelve', () => {
  test.each(['sin-cobros', 'ya-devuelto', 'excede', 'sin-monto', 'sin-motivo'] as const)(
    '%s dice algo accionable y sin internals', (motivo) => {
      const m = porQueNoSeDevuelve(motivo);
      expect(m.length).toBeGreaterThan(10);
      const palabras = m.toLowerCase().split(/[^a-záéíóúñ]+/);
      for (const prohibida of ['refund', 'amount', 'null', 'undefined', 'endpoint', 'query']) {
        expect(palabras).not.toContain(prohibida);
      }
    },
  );
});

// EL AVISO QUE EVITA QUE EL ALMACÉN CUADRE MAL SIN QUE NADIE SEPA POR QUÉ.
//
// Cancelar un renglón que ya salió a cocina baja el total pero NO devuelve el insumo, porque se
// gastó. Decirlo antes de confirmar es la diferencia entre una merma explicada y una inexplicable.
describe('avisoDeInventario', () => {
  test('el que ya salió a cocina avisa que el ingrediente no vuelve', () => {
    const m = avisoDeInventario(true);
    expect(m).toMatch(/no vuelve/i);
  });

  test('el que no salió avisa que sí vuelve', () => {
    const m = avisoDeInventario(false);
    expect(m).toMatch(/vuelve al almacén/i);
    expect(m).not.toMatch(/no vuelve/i);
  });

  test('los dos avisos son distintos: si dijeran lo mismo, el aviso no informaría nada', () => {
    expect(avisoDeInventario(true)).not.toBe(avisoDeInventario(false));
  });
});
