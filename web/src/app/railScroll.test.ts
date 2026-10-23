import { expect, test } from 'vitest';
import { railScrollFor } from './railScroll';

// Una barra de 300 px con opciones de 52 px (44 + separación) que empiezan cada 52: la activa se lleva
// a la vista sin partir la de arriba ni la activa (spec 045, B10). La geometría se finge: jsdom no mide.
function rail(scrollTop: number, n = 12, rowH = 52, clientHeight = 300) {
  const list = { scrollTop, clientHeight, getBoundingClientRect: () => ({ top: 100 }) } as unknown as HTMLElement;
  const items = Array.from({ length: n }, (_, i) => ({
    getBoundingClientRect: () => ({ top: 100 + i * rowH - list.scrollTop, height: 44 }),
  })) as unknown as HTMLElement[];
  Object.defineProperty(list, 'children', { value: items });
  return { list, items };
}

test.each([
  // [scroll inicial, índice activo, scroll esperado]
  [0, 0, 0],        // ya se ve: no se mueve
  [0, 4, 0],        // 4 empieza en 208 y acaba en 252 < 300: se ve
  [0, 9, 260],      // acaba en 512: hace falta 212; el primer inicio que la deja entera es 260, no 212
  [0, 11, 364],     // la última: acaba en 616, hace falta 316 → 364
  [312, 2, 104],    // arriba de lo visible: queda primera
])('desde %i con la opción %i activa → %i', (from, idx, want) => {
  const { list, items } = rail(from);
  const got = railScrollFor(list, items[idx]);
  expect(got).toBe(want);
  expect(got % 52, 'no empieza en el inicio de una opción').toBe(0);
});
