import { describe, expect, it } from 'vitest';
import { entregados, faltante, pendientes } from './entrega';
import type { BoardLine, BoardOrder } from '../../types/pos';

const linea = (l: Partial<BoardLine>): BoardLine => ({
  id: 1, name: 'Alitas', qty: '5', delivered: '0', ...l,
});

const pedido = (lines: BoardLine[]): BoardOrder => ({
  id: 1, number: 1, folioName: 'Tigre', status: 'abierta', serviceType: 'mostrador',
  deliveryPlatformId: null, customerName: null, total: '0', outstanding: '0',
  currency: 'MXN', paid: true, enPreparacion: false, renglones: 1,
  openedAt: '2026-08-31T18:00:00Z', lines,
});

describe('faltante', () => {
  it('resta lo entregado de lo pedido', () => {
    expect(faltante(linea({ qty: '5', delivered: '3' }))).toBe(2);
  });
  it('es cero cuando ya salió todo', () => {
    expect(faltante(linea({ qty: '5', delivered: '5' }))).toBe(0);
  });
  it('maneja fracciones', () => {
    expect(faltante(linea({ qty: '1.5', delivered: '0.75' }))).toBe(0.75);
  });
});

describe('pendientes', () => {
  it('deja fuera lo ya entregado', () => {
    const p = pedido([
      linea({ id: 1, qty: '5', delivered: '5' }),
      linea({ id: 2, qty: '2', delivered: '0' }),
      linea({ id: 3, qty: '5', delivered: '3' }),
    ]);
    expect(pendientes(p).map((l) => l.id)).toEqual([2, 3]);
  });
});

describe('entregados', () => {
  it('cuenta renglones completos, no piezas', () => {
    // 13 de 14 piezas, pero 1 de 2 productos: lo segundo es lo que se verifica contra la charola.
    const p = pedido([
      linea({ id: 1, qty: '5', delivered: '5' }),
      linea({ id: 2, qty: '9', delivered: '8' }),
    ]);
    expect(entregados(p)).toBe(1);
  });
  it('un pedido sin renglones no tiene nada entregado', () => {
    expect(entregados(pedido([]))).toBe(0);
  });
});

// LA PANTALLA DEL MOSTRADOR NO SE PUEDE CAER POR LO QUE VENGA EN UNA RESPUESTA.
//
// El 2026-09-13 el servidor mandó `lines: null` en un pedido cuya única línea se había cancelado
// —un slice nil de Go se serializa como `null`, no como `[]`— y estas dos funciones, que corren al
// PINTAR cada tarjeta, reventaron con `Cannot read properties of null (reading 'filter')`. Se cayó
// el tablero entero y el mostrador se quedó con «La pantalla no se pudo mostrar»; reiniciar no
// servía, porque el dato volvía igual.
//
// El servidor ya no lo manda y eso lo fija `TestElTableroNuncaMandaRenglonesNulos`. Esto es la otra
// mitad: que ninguna respuesta pueda volver a tumbar la pantalla donde se vende. Un pedido sin
// renglones se pinta vacío, que es la verdad.
describe('un pedido sin renglones no tumba la pantalla', () => {
  // `as BoardOrder` a propósito: el tipo dice que `lines` es un arreglo, y el defecto fue
  // exactamente que la realidad no lo era. Un test que respete el tipo no puede reproducirlo.
  const sinRenglones = { ...pedido([]), lines: null } as unknown as BoardOrder;

  it('pendientes devuelve vacío en vez de reventar', () => {
    expect(pendientes(sinRenglones)).toEqual([]);
  });

  it('entregados devuelve cero en vez de reventar', () => {
    expect(entregados(sinRenglones)).toBe(0);
  });

  it('y con el campo ausente del todo, igual', () => {
    const sinCampo = { ...pedido([]) } as Partial<BoardOrder>;
    delete sinCampo.lines;
    expect(pendientes(sinCampo as BoardOrder)).toEqual([]);
    expect(entregados(sinCampo as BoardOrder)).toBe(0);
  });
});
