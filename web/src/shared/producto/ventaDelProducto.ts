import type { AdminProduct, ProductSales } from '../../api/admin';
import { montoTecleado } from '../../domain/numeros';

// El tope de un monto individual del servidor (domain.MaxMoney).
const MAX_MONEY = 10_000_000;

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
  return precioDe(v.precioWeb) !== undefined;
}

// precioDe lee el texto como precio: mayor que cero, a lo más dos decimales (más decimales se
// rechazan, no se redondean en silencio) y dentro del tope del servidor.
function precioDe(texto: string): number | undefined {
  const t = texto.trim();
  if (!/^\d+(\.\d{1,2})?$/.test(t)) return undefined;
  const n = montoTecleado(t);
  return n !== undefined && n > 0 && n <= MAX_MONEY ? n : undefined;
}

// ventaParaGuardar es lo que viaja al servidor. Apagar la web no borra el precio ni la descripción:
// con la web apagada y el campo vacío o mal escrito (oculto para quien opera), se manda el precio que
// ya estaba guardado en vez de borrarlo o de mandar algo que el servidor rechazaría sin pista.
export function ventaParaGuardar(v: VentaDelProducto, guardado: string | null): ProductSales {
  const n = precioDe(v.precioWeb);
  let webPrice: string | null = null;
  if (!v.usarMostrador) webPrice = n !== undefined ? n.toFixed(2) : v.web ? null : guardado;
  return {
    counter: v.counter,
    web: v.web,
    webPrice,
    allowOversell: v.allowOversell,
    description: v.description.trim(),
  };
}
