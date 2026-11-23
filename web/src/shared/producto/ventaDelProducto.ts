import type { AdminProduct, ProductSales } from '../../api/admin';
import { montoTecleado } from '../../domain/numeros';

// Lo que la ficha edita de la venta del producto (spec 092). `usarMostrador` y `precioWeb` son el
// estado de la pantalla; lo que viaja es `ProductSales`.
export interface VentaDelProducto {
  counter: boolean;
  web: boolean;
  usarMostrador: boolean;
  precioWeb: string;
  allowOversell: boolean;
  description: string;
}

// ventaInicial lee lo guardado con los defaults de un producto que nunca se configuró: en los dos
// lugares, al precio de mostrador y con «vender sin existencia».
export function ventaInicial(p: AdminProduct | null): VentaDelProducto {
  const precio = p?.webPrice ?? null;
  return {
    counter: p?.soldAtCounter ?? true,
    web: p?.soldOnWeb ?? true,
    usarMostrador: precio == null,
    precioWeb: precio ?? '',
    allowOversell: p?.allowOversell ?? true,
    description: p?.description ?? '',
  };
}

// precioWebValido: con «usar el de mostrador» apagado, el precio es obligatorio y mayor que cero.
// Vacío nunca significa cero.
export function precioWebValido(v: VentaDelProducto): boolean {
  if (v.usarMostrador) return true;
  const n = montoTecleado(v.precioWeb);
  return n !== undefined && n > 0;
}

// ventaParaGuardar es lo que viaja al servidor. Apagar la web no borra el precio ni la descripción.
export function ventaParaGuardar(v: VentaDelProducto): ProductSales {
  return {
    counter: v.counter,
    web: v.web,
    webPrice: v.usarMostrador ? null : (montoTecleado(v.precioWeb) ?? null),
    allowOversell: v.allowOversell,
    description: v.description.trim(),
  };
}
