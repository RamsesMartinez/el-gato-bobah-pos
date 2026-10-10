import { describe, expect, it } from 'vitest';
import { repartoParejo, validarAjustado } from './propinas';

// Los centavos no se reparten (decisión del dueño, 2026-10-09): pesos enteros por persona y el
// sobrante se queda pendiente. La pantalla dice lo mismo que va a cobrar el servidor.
describe('repartoParejo', () => {
  it('reparte pesos enteros y deja el sobrante', () => {
    expect(repartoParejo(186.5, 3)).toEqual({ cada: 62, sobrante: 0.5 });
    expect(repartoParejo(187, 3)).toEqual({ cada: 62, sobrante: 1 });
  });
  it('no alcanza un peso por persona: no hay reparto', () => {
    expect(repartoParejo(2.5, 3)).toBeNull();
  });
  it('sin personas no hay reparto', () => {
    expect(repartoParejo(100, 0)).toBeNull();
  });
});

describe('validarAjustado', () => {
  it('acepta pesos enteros que no pasan del pendiente', () => {
    expect(validarAjustado(100, ['60', '40'])).toBeNull();
  });
  it('un campo vacío no es cero: falta capturarlo', () => {
    expect(validarAjustado(100, ['60', ''])).toBe('Escribe cuánto recibe cada persona');
  });
  it('rechaza centavos', () => {
    expect(validarAjustado(100, ['40.50'])).toBe('La propina se entrega en pesos enteros');
  });
  it('rechaza pasarse del pendiente', () => {
    expect(validarAjustado(100.5, ['101'])).toBe('No hay tanta propina por entregar');
  });
  it('rechaza cero', () => {
    expect(validarAjustado(100, ['0'])).toBe('Escribe cuánto recibe cada persona');
  });
});
