import { describe, expect, it } from 'vitest';
import { cabeOtra, cantidadDe, sumarUna, type Picks } from './seleccionModificadores';
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
