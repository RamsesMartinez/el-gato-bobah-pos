import { describe, it, expect } from 'vitest';
import { parseNumero, parseMonto, round2, round4 } from './numeros';

// Leer un número que tecleó una persona es la operación más repetida del sistema y la que más veces
// se reimplementó: había cuatro redondeos y dos parsers distintos del MISMO campo. Aquí es una sola
// implementación, y la regla de lint impide que aparezca la quinta.

describe('parseNumero', () => {
  it('distingue AUSENTE de cero: no es lo mismo no haber escrito nada que escribir un 0', () => {
    // El campo "con cuánto paga" vacío significa "pagó justo". Tratarlo como $0 recibidos deja el
    // aviso de efectivo insuficiente encendido para siempre y el botón de cobrar muerto.
    expect(parseNumero('')).toEqual({ estado: 'ausente' });
    expect(parseNumero('   ')).toEqual({ estado: 'ausente' });
    expect(parseNumero('0')).toEqual({ estado: 'valido', valor: 0 });
  });

  it('rechaza la coma de millar en vez de leerla como 1', () => {
    // parseFloat('1,000') devuelve 1 — y es finito, así que ni siquiera las guardas de
    // Number.isFinite lo atrapan. En una tableta el operador teclea la coma por costumbre.
    expect(parseNumero('1,000')).toEqual({ estado: 'invalido', motivo: 'formato' });
    expect(parseNumero('1,000.50')).toEqual({ estado: 'invalido', motivo: 'formato' });
  });

  it('rechaza el texto con cola en vez de quedarse con el principio', () => {
    // parseFloat('12kg') devuelve 12: la unidad tecleada por error se descarta en silencio y la
    // cantidad entra como si estuviera bien.
    expect(parseNumero('12kg')).toEqual({ estado: 'invalido', motivo: 'formato' });
    expect(parseNumero('abc')).toEqual({ estado: 'invalido', motivo: 'formato' });
    expect(parseNumero('.')).toEqual({ estado: 'invalido', motivo: 'formato' });
    expect(parseNumero('1.2.3')).toEqual({ estado: 'invalido', motivo: 'formato' });
  });

  it('rechaza el negativo, y lo dice por su nombre', () => {
    expect(parseNumero('-50')).toEqual({ estado: 'invalido', motivo: 'negativo' });
  });

  it('no redondea: hay campos con cuatro decimales, como el contenido de un empaque', () => {
    expect(parseNumero('33.3333')).toEqual({ estado: 'valido', valor: 33.3333 });
    expect(parseNumero(' 100.5 ')).toEqual({ estado: 'valido', valor: 100.5 });
  });
});

describe('parseMonto', () => {
  it('es parseNumero redondeado a dos decimales, que es la escala del dinero', () => {
    expect(parseMonto('33.335')).toEqual({ estado: 'valido', valor: 33.34 });
    expect(parseMonto('')).toEqual({ estado: 'ausente' });
    expect(parseMonto('1,000')).toEqual({ estado: 'invalido', motivo: 'formato' });
  });
});

describe('round2 y round4', () => {
  it('redondean en la frontera sin caer del lado malo', () => {
    // Math.round(1.005 * 100) da 100 porque 1.005 no es exacto en binario: sin el EPSILON, un peso
    // con cinco milésimas baja en vez de subir, y eso en una suma de corte no se explica.
    expect(round2(1.005)).toBe(1.01);
    expect(round2(0.1 + 0.2)).toBe(0.3);
    expect(round2(33.335)).toBe(33.34);
  });

  it('round4 es la escala del stock, no la del dinero', () => {
    // numeric(14,4) en el almacén: redondear a 2 ahí corrompe el inventario de lo que se vende por
    // gramo.
    expect(round4(0.12345)).toBe(0.1235);
    expect(round4(2.5)).toBe(2.5);
  });
});
