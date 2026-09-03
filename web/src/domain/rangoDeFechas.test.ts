import { describe, expect, test } from 'vitest';
import {
  MAX_DIAS_RANGO, diaValido, diasDelRango, mensajeDeRango, validarRango,
} from './rangoDeFechas';

// EL 31 DE FEBRERO NO EXISTE, Y ACEPTARLO ES CONSULTAR OTRO DÍA.
//
// `new Date('2026-02-31')` no falla: rueda al 3 de marzo. Un campo que lo acepta manda al servidor
// una fecha distinta de la que el operador tecleó, y la pantalla se ve perfecta contestando un
// periodo que nadie pidió — el modo de falla que el principio V nombra.
describe('diaValido', () => {
  test.each(['2026-08-01', '2024-02-29', '2026-12-31'])('acepta %s', (v) => {
    expect(diaValido(v)).toBe(true);
  });

  test.each([
    ['2026-02-31', 'un día que no existe'],
    ['2026-13-01', 'un mes que no existe'],
    ['2026-00-10', 'el mes cero'],
    ['2026-8-1', 'sin ceros a la izquierda'],
    ['01/08/2026', 'con diagonales'],
    ['', 'vacío'],
    ['hoy', 'una palabra'],
    ['2025-02-29', 'el 29 de un febrero que no es bisiesto'],
  ])('rechaza %s (%s)', (v) => {
    expect(diaValido(v)).toBe(false);
  });
});

describe('diasDelRango', () => {
  // Inclusivo en los dos extremos: del 1 al 1 es UN día, no cero. Con la cuenta exclusiva, un
  // "rango de hoy a hoy" mediría cero días y el tope de 366 dejaría pasar uno de 367.
  test('del mismo día a sí mismo es un día', () => {
    expect(diasDelRango('2026-08-01', '2026-08-01')).toBe(1);
  });

  test('cuenta los dos extremos', () => {
    expect(diasDelRango('2026-08-01', '2026-08-31')).toBe(31);
  });

  // Cruza el cambio de horario de verano de México sin perder ni ganar un día: contar con Date
  // local restaría una hora y el redondeo se comería un día en un rango largo.
  test('cruzar el cambio de horario no mueve la cuenta', () => {
    expect(diasDelRango('2026-10-25', '2026-10-27')).toBe(3);
  });

  test('invertido cuenta cero', () => {
    expect(diasDelRango('2026-08-31', '2026-08-01')).toBe(0);
  });
});

describe('validarRango', () => {
  test('un rango normal pasa', () => {
    expect(validarRango('2026-08-01', '2026-08-15')).toBeNull();
  });

  // Media fecha NO es un rango. Mandarlo con una sola haría que el servidor conteste el default y
  // la pantalla enseñe un periodo distinto del que se está capturando.
  test.each([
    ['2026-08-01', ''],
    ['', '2026-08-01'],
    ['', ''],
  ])('media fecha (%s, %s) queda incompleta', (a, b) => {
    expect(validarRango(a, b)).toBe('incompleto');
  });

  // Invertido devolvería CERO filas sin error, y el operador creería que no vendió.
  test('invertido se rechaza', () => {
    expect(validarRango('2026-08-31', '2026-08-01')).toBe('invertido');
  });

  test('una fecha que no existe se rechaza', () => {
    expect(validarRango('2026-02-31', '2026-03-05')).toBe('malformado');
  });

  // El tope es el MISMO que el del servidor. Si el front dejara pedir 400 días, el rebote llegaría
  // con el dedo ya levantado del botón y sin decir qué corregir.
  test('el tope son 366 días, y el 366 pasa', () => {
    expect(validarRango('2026-01-01', '2026-12-31')).toBeNull(); // 365
    expect(diasDelRango('2025-01-01', '2026-01-01')).toBe(MAX_DIAS_RANGO);
    expect(validarRango('2025-01-01', '2026-01-01')).toBeNull();
    expect(validarRango('2025-01-01', '2026-01-02')).toBe('demasiados-dias');
  });
});

// El texto es para quien atiende el negocio, no para quien lo programó: sin nombres de parámetros,
// de endpoints ni de columnas.
describe('mensajeDeRango', () => {
  test.each(['incompleto', 'malformado', 'invertido', 'demasiados-dias'] as const)(
    '%s dice algo accionable y sin internals', (motivo) => {
      const m = mensajeDeRango(motivo);
      expect(m.length).toBeGreaterThan(10);
      expect(m).not.toMatch(/from|to|preset|query|param|endpoint|null|undefined/i);
    },
  );
});
