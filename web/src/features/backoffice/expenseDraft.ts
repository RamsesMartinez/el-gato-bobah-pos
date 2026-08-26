// Reglas puras del borrador de gasto: convertir lo que dice un documento a lo que el almacén y
// la contabilidad necesitan. Viven aparte del diálogo para poder probarse sin montar la UI.

// Alias de unidad tal como los imprimen los documentos → código de la tabla de unidades.
// Solo lo que se convierte con certeza: "hojas", "HD" o "rollos" no son unidades del almacén y
// quedan fuera a propósito.
const UNIT_ALIASES: Record<string, string> = {
  g: 'g', gr: 'g', grs: 'g', gramo: 'g', gramos: 'g',
  kg: 'kg', k: 'kg', kgs: 'kg', kilo: 'kg', kilos: 'kg', kilogramo: 'kg', kilogramos: 'kg',
  ml: 'ml', mls: 'ml', mililitro: 'ml', mililitros: 'ml',
  l: 'l', lt: 'l', lts: 'l', litro: 'l', litros: 'l',
  pieza: 'pieza', piezas: 'pieza', pz: 'pieza', pzs: 'pieza', pza: 'pieza', pzas: 'pieza',
  unidad: 'pieza', unidades: 'pieza',
};

// packToBase convierte el contenido de UNA unidad de compra ("2.26 kg") a la unidad BASE del
// almacén ("2260"). Es imprescindible: el extractor lee "2.26K MOZZAR" y pasar ese 2.26 tal cual
// metería 2.26 gramos de queso al inventario en vez de 2260.
//
// Devuelve '' cuando la unidad no se reconoce, en vez de arriesgar la escala: un contenido en
// blanco lo completa el operador; uno equivocado corrompe el stock en silencio.
export function packToBase(
  packQty: string,
  packUnit: string,
  units: { code: string; toBase: string }[],
): string {
  const qty = parseFloat(packQty);
  if (!packQty || !Number.isFinite(qty) || qty <= 0) return '';
  const key = packUnit.trim().toLowerCase().replace(/[^a-záéíóúñ]/g, '');
  const code = UNIT_ALIASES[key];
  if (!code) return '';
  const unit = units.find((u) => u.code === code);
  if (!unit) return '';
  const factor = parseFloat(unit.toBase);
  if (!Number.isFinite(factor) || factor <= 0) return '';
  // 4 decimales = la escala de numeric(14,4) del almacén; Number() recorta ceros de relleno.
  return String(Number((qty * factor).toFixed(4)));
}

// lineAmount deriva el importe del renglón cuando el documento no lo imprime: hay pedidos que
// solo publican el precio "c/u" y nunca el total de la línea (3 piezas a 17.00 no traen 51.00 en
// ninguna parte del papel). Sin esto el renglón se guardaría en 0 y el gasto no cuadraría.
//
// Es la misma regla que domain.EffectiveAmount en el backend, a propósito: el operador tiene que
// ver en el formulario el mismo número que se va a guardar.
// splitTotals separa lo que es gasto del local de lo que venía en el mismo ticket pero es de la
// casa. Son dos números distintos y confundirlos infla los gastos del negocio con el shampoo.
export function splitTotals(items: { amount: string; personal: boolean }[]): {
  local: number; personal: number;
} {
  let local = 0;
  let personal = 0;
  for (const it of items) {
    const n = parseFloat(it.amount);
    if (!Number.isFinite(n)) continue;
    if (it.personal) personal += n;
    else local += n;
  }
  // Redondeo a 2dp en la frontera: sumar flotantes deja 0.30000000000000004 y el cuadre contra
  // el total del documento fallaría por un centavo fantasma.
  return { local: Number(local.toFixed(2)), personal: Number(personal.toFixed(2)) };
}

export function lineAmount(amount: string, unitPrice: string, qty: string): string {
  if (amount.trim() !== '') return amount;
  const p = parseFloat(unitPrice);
  if (!Number.isFinite(p)) return '';
  const q = parseFloat(qty);
  const n = p * (Number.isFinite(q) && q > 0 ? q : 1);
  return n.toFixed(2);
}
