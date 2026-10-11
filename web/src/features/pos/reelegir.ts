import type { Menu, MenuProduct } from '../../types/pos';

// productoParaReelegir dice qué producto vuelve a abrir su hoja de extras después de un rechazo por
// menú viejo: solo uno que sigue a la venta en el menú recién traído y que tiene extras que elegir.
export function productoParaReelegir(menu: Menu | undefined, productId: number): MenuProduct | null {
  const p = menu?.products.find((x) => x.id === productId);
  return p && p.groups.length > 0 ? p : null;
}
