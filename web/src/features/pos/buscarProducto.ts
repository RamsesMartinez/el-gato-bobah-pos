import type { MenuProduct } from '../../types/pos';
import { matchedAlias, normalize } from '../../utils/format';

// Buscar en la pantalla de venta, por nombre Y por código.
//
// El código importa por el flujo real del local: se toma el pedido en libreta y se transcribe en la
// tableta del mostrador, donde escribir es tocar un teclado en pantalla. Un código de tres o cuatro
// letras cuesta menos taps que bajar por categorías o que teclear el nombre completo.
//
// Las coincidencias por NOMBRE van primero: quien escribe "bone" busca el boneless, no un producto
// cuyo código casualmente lo contenga. Después las de otro nombre (spec 063): «café frío» trae «Ice
// Coffee», pero detrás de lo que se llama así.
export function buscarProductos(productos: MenuProduct[], busqueda: string): MenuProduct[] {
  // trim antes de normalizar: `normalize` no lo hace, y una búsqueda de puros espacios pasaría el
  // filtro sin coincidir con nada — la rejilla se vaciaría por un roce del dedo en el campo.
  const q = normalize(busqueda.trim());
  if (!q) return productos;

  const porNombre: MenuProduct[] = [];
  const porOtroNombre: MenuProduct[] = [];
  const porCodigo: MenuProduct[] = [];
  for (const p of productos) {
    if (normalize(p.name).includes(q)) porNombre.push(p);
    else if (aliasCoincidente(p, busqueda)) porOtroNombre.push(p);
    else if (p.sku && normalize(p.sku).includes(q)) porCodigo.push(p);
  }
  return [...porNombre, ...porOtroNombre, ...porCodigo];
}

const contiene = (texto: string, busqueda: string) => normalize(texto).includes(normalize(busqueda.trim()));

// aliasCoincidente es el otro nombre por el que salió un producto, para señalarlo en el tile; nada si
// salió por su nombre (o no salió).
export function aliasCoincidente(p: MenuProduct, busqueda: string): string | undefined {
  if (!busqueda.trim() || contiene(p.name, busqueda)) return undefined;
  return matchedAlias(p.altNames, busqueda, contiene);
}
