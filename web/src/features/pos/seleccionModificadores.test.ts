import { describe, expect, it } from 'vitest';
import { alTocarUnaSola, cabeOtra, cantidadDe, sumarUna, type Picks } from './seleccionModificadores';
import type { MenuOption } from '../../types/pos';

const salsa = (id: number, maxPerLine = 2): MenuOption =>
  ({ id, name: `Salsa ${id}`, priceDelta: '0', maxPerLine, favorite: false });

describe('repetir la misma opción', () => {
  // El caso que motivó todo: el grupo "Salsas" pide 2 y el cliente quiere las dos de mango.
  it('con cupo en el grupo y la opción permitiéndolo, cabe otra', () => {
    const picks: Picks = { 10: 1 };
    expect(cabeOtra(picks, salsa(10), 2)).toBe(true);
  });

  it('al llenar el grupo ya no cabe, aunque la opción lo permita', () => {
    expect(cabeOtra({ 10: 2 }, salsa(10), 2)).toBe(false);
    expect(cabeOtra({ 10: 1, 11: 1 }, salsa(10), 2)).toBe(false);
  });

  // 818 de las 1092 opciones tienen maxPerLine 1: para ellas nada cambia y el "+" no debe salir.
  it('una opción que no admite repetirse nunca ofrece otra', () => {
    expect(cabeOtra({ 10: 1 }, salsa(10, 1), 3)).toBe(false);
  });

  // El "+" solo suma; nunca aparece sobre algo que no está elegido, porque ahí el toque normal
  // ya hace el trabajo.
  it('sobre una opción no elegida no ofrece nada', () => {
    expect(cabeOtra({}, salsa(10), 2)).toBe(false);
  });

  it('un grupo de una sola selección nunca ofrece repetir', () => {
    expect(cabeOtra({ 10: 1 }, salsa(10), 1)).toBe(false);
  });

  it('sumarUna incrementa solo esa opción y deja el resto igual', () => {
    expect(sumarUna({ 10: 1, 11: 1 }, 10)).toEqual({ 10: 2, 11: 1 });
  });

  it('cantidadDe cuenta lo elegido de una opción', () => {
    expect(cantidadDe({ 10: 2 }, 10)).toBe(2);
    expect(cantidadDe({}, 10)).toBe(0);
  });
});

// EL ADEREZO DE CORTESÍA MARCADO POR ERROR TENÍA QUE PODER QUITARSE.
//
// El grupo dice "opcional" y aun así, una vez tocado, no había manera de dejarlo vacío: el toque
// repetido volvía a elegir lo mismo. La línea se iba a cocina con un aderezo que el cliente no
// pidió, y la única salida era borrar el renglón entero y recapturarlo.
describe('tocar una opción en un grupo de una sola', () => {
  it('en un grupo opcional, tocar la elegida la quita', () => {
    expect(alTocarUnaSola({ 30: 1 }, 30, 0)).toEqual({});
  });

  it('en un grupo opcional, tocar otra la reemplaza en un toque', () => {
    expect(alTocarUnaSola({ 30: 1 }, 31, 0)).toEqual({ 31: 1 });
  });

  it('sobre un grupo vacío elige, sin importar el mínimo', () => {
    expect(alTocarUnaSola({}, 30, 0)).toEqual({ 30: 1 });
    expect(alTocarUnaSola({}, 30, 1)).toEqual({ 30: 1 });
  });

  // En el obligatorio el toque repetido NO vacía: hay que elegir algo de todos modos, así que
  // vaciarlo solo produce una línea inválida que el operador tiene que deshacer. Cambiar de opción
  // sigue costando un toque, que es lo que de verdad se necesita ahí.
  it('en un grupo obligatorio, tocar la elegida la deja puesta', () => {
    expect(alTocarUnaSola({ 40: 1 }, 40, 1)).toEqual({ 40: 1 });
  });

  it('en un grupo obligatorio, tocar otra la reemplaza', () => {
    expect(alTocarUnaSola({ 40: 1 }, 41, 1)).toEqual({ 41: 1 });
  });
});
