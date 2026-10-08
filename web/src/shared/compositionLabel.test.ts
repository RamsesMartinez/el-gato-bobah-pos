import { describe, it, expect } from 'vitest';
import { friendlyQuantity } from './compositionLabel';
import type { Unit } from '../api/backoffice';

const u = (id: number, code: string, kind: string, toBase: string) => ({ id, code, name: code, kind, toBase }) as Unit;
const units = [u(1, 'g', 'masa', '1'), u(2, 'kg', 'masa', '1000'), u(3, 'ml', 'volumen', '1'), u(4, 'l', 'volumen', '1000'), u(5, 'pieza', 'pieza', '1')];

// FUDO guarda en kilos y litros: «0.215 kg» de hielo se lee mal y se corrige peor que «215 g».
describe('friendlyQuantity', () => {
  it('menos de un kilo o un litro se muestra en gramos o mililitros', () => {
    expect(friendlyQuantity(0.215, 2, units)).toEqual({ text: '215', unitId: 1 });
    expect(friendlyQuantity(0.3, 4, units)).toEqual({ text: '300', unitId: 3 });
  });
  it('un kilo o más se queda en kilos', () => {
    expect(friendlyQuantity(1.5, 2, units)).toEqual({ text: '1.5', unitId: 2 });
  });
  it('sin el catálogo de unidades no cambia nada', () => {
    expect(friendlyQuantity(0.215, 2, [])).toEqual({ text: '0.215', unitId: 2 });
  });
  it('la conversión no deja basura de punto flotante', () => {
    expect(friendlyQuantity(0.047, 2, units)).toEqual({ text: '47', unitId: 1 });
  });
});
