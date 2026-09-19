import { describe, expect, it } from 'vitest';

import { descuentoDeLaCuenta } from './descuento';

describe('descuentoDeLaCuenta', () => {
  // El campo vacío es AUSENCIA, no cero. Es el defecto que este repo ya pagó una vez: borrar un
  // campo para reescribirlo apagaba una protección a media captura.
  it('un campo vacío es sin descuento, no un descuento de cero', () => {
    const d = descuentoDeLaCuenta('', 'monto', 385);
    expect(d.monto).toBe(0);
    expect(d.malEscrito).toBe(false);
    expect(d.paraElServidor).toBeUndefined();
  });

  it('un monto se resta tal cual', () => {
    const d = descuentoDeLaCuenta('50', 'monto', 385);
    expect(d.monto).toBe(50);
    expect(d.paraElServidor).toEqual({ discountAmount: 50 });
  });

  // El porcentaje viaja como porcentaje: lo resuelve el servidor contra SU subtotal. Lo que se
  // calcula aquí es solo lo que la pantalla pinta.
  it('un porcentaje se pinta en pesos pero viaja como porcentaje', () => {
    const d = descuentoDeLaCuenta('20', 'pct', 385);
    expect(d.monto).toBe(77);
    expect(d.paraElServidor).toEqual({ discountPercent: 20 });
  });

  // "1,000" es lo que el teclado decimal de Android ofrece y lo que una persona teclea por
  // costumbre. `parseFloat` devolvería 1 y descontaría un peso en vez de mil: se rechaza.
  it('lo que no es un número bloquea, no cae a cero', () => {
    const d = descuentoDeLaCuenta('1,000', 'monto', 5000);
    expect(d.malEscrito).toBe(true);
    expect(d.monto).toBe(0);
    expect(d.paraElServidor).toBeUndefined();
  });

  it('un descuento mayor que la cuenta se marca y no se manda', () => {
    const d = descuentoDeLaCuenta('400', 'monto', 385);
    expect(d.excede).toBe(true);
    expect(d.paraElServidor).toBeUndefined();
  });

  it('más de 100% se marca igual que un monto que excede', () => {
    const d = descuentoDeLaCuenta('120', 'pct', 385);
    expect(d.excede).toBe(true);
    expect(d.paraElServidor).toBeUndefined();
  });

  it('un descuento del 100% deja la cuenta en cero y es válido', () => {
    const d = descuentoDeLaCuenta('100', 'pct', 385);
    expect(d.monto).toBe(385);
    expect(d.excede).toBe(false);
  });

  // El redondeo es a centavos y del lado de la pantalla solo para PINTAR: el servidor vuelve a
  // resolverlo. Que coincidan es lo que evita que el operador vea una cifra y se cobre otra.
  it('el porcentaje con residuo se pinta redondeado a centavos', () => {
    expect(descuentoDeLaCuenta('33', 'pct', 100.05).monto).toBe(33.02);
  });

  it('un descuento negativo no existe', () => {
    expect(descuentoDeLaCuenta('-5', 'monto', 385).malEscrito).toBe(true);
  });
});
