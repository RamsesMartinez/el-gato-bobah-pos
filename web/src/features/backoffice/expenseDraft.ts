import { parseMonto, parseNumero, round2 } from '../../domain/numeros';

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
  // parseNumero y no parseFloat: `parseFloat('1,000')` devuelve 1 y ES finito, así que la guarda de
  // abajo no lo atrapaba — el operador teclea la coma de millar y el contenido del empaque entra
  // mil veces más chico. Lo mismo con '12kg', que se leía como 12.
  const cant = parseNumero(packQty);
  if (cant.estado !== 'valido' || cant.valor <= 0) return '';
  const qty = cant.valor;
  const key = packUnit.trim().toLowerCase().replace(/[^a-záéíóúñ]/g, '');
  const code = UNIT_ALIASES[key];
  if (!code) return '';
  const unit = units.find((u) => u.code === code);
  if (!unit) return '';
  // Este viene del servidor, no de un teclado, pero se lee igual: una sola forma de leer números.
  const conv = parseNumero(unit.toBase);
  if (conv.estado !== 'valido' || conv.valor <= 0) return '';
  const factor = conv.valor;
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
    const importe = parseMonto(it.amount);
    if (importe.estado !== 'valido') continue;
    const n = importe.valor;
    if (it.personal) personal += n;
    else local += n;
  }
  // Redondeo a 2dp en la frontera: sumar flotantes deja 0.30000000000000004 y el cuadre contra
  // el total del documento fallaría por un centavo fantasma.
  return { local: Number(local.toFixed(2)), personal: Number(personal.toFixed(2)) };
}

export function lineAmount(amount: string, unitPrice: string, qty: string): string {
  if (amount.trim() !== '') return amount;
  const precio = parseMonto(unitPrice);
  if (precio.estado !== 'valido') return '';
  const cant = parseNumero(qty);
  const n = precio.valor * (cant.estado === 'valido' && cant.valor > 0 ? cant.valor : 1);
  return n.toFixed(2);
}

// Reglas de dinero de una compra (spec 038, FR-004a): cada renglón de mercancía lleva importe
// salvo un regalo, y los renglones no pasan del total pagado; lo que falta queda como «otros» del
// gasto. El servidor las vuelve a validar: aquí es para decirlo en el pie antes del 400.
export interface PurchaseMoneyLine {
  amount: string;
  goods: boolean; // renglón de almacén
  gift: boolean;
  // Fuera del gasto: lo excluido (de la casa) no suma al importe del negocio.
  excluded?: boolean;
}
export interface PurchaseMoney {
  linesSum: number;
  missingAmount: number; // renglones sin importe que cuentan: ni regalo ni excluidos
  over: boolean; // los renglones pasan del total pagado
  others: number; // lo que queda como «otros» del gasto
}
export function purchaseMoney(lines: PurchaseMoneyLine[], total: number): PurchaseMoney {
  let sum = 0;
  let missingAmount = 0;
  for (const l of lines) {
    if (l.excluded) continue;
    const m = parseMonto(l.amount);
    const v = m.estado === 'valido' ? m.valor : 0;
    if (!l.gift && !(v > 0)) missingAmount++;
    sum += v;
  }
  const linesSum = round2(sum);
  return { linesSum, missingAmount, over: linesSum > round2(total), others: Math.max(0, round2(total - linesSum)) };
}

// blankLine: el renglón que la captura con mercancía agrega y nadie llenó. No se manda: en 0 la base
// lo rechaza, y obligar a borrarlo para guardar el gasto es un toque que no aporta nada.
export function blankLine(l: { description: string; rawName: string; itemId: number | null; amount: string }): boolean {
  // Un importe escrito mal es un dato: el renglón se queda y la pantalla lo marca.
  const m = parseMonto(l.amount);
  const noAmount = !l.amount.trim() || (m.estado === 'valido' && m.valor === 0);
  return !l.description.trim() && !l.rawName.trim() && !l.itemId && noAmount;
}

// baseOfLine es lo que un renglón suma en la unidad base del artículo: cantidad × lo que trae su
// empaque (o el factor de su unidad). null cuando falta alguno de los dos: un «= 0 g» engañaría.
export function baseOfLine(qty: string, factor: string | null | undefined): number | null {
  const q = parseNumero(qty);
  const f = parseNumero(factor ?? '');
  if (q.estado !== 'valido' || f.estado !== 'valido' || q.valor <= 0 || f.valor <= 0) return null;
  return Number((q.valor * f.valor).toFixed(4));
}

// Spec 058. densityFactor es cuánto suma en la unidad base UNA unidad de compra de la otra clase
// (kg de un artículo en ml): la misma regla que domain.BaseQty — masa → volumen divide entre la
// densidad, volumen → masa multiplica. null cuando no aplica (misma clase, pieza, longitud) o falta
// la densidad: sin ella el servidor rechaza el renglón y la pantalla no debe inventar un número.
export function densityFactor(buyKind: string, baseKind: string, toBase: string, density: string | null | undefined): number | null {
  const d = parseNumero(density ?? '');
  const f = parseNumero(toBase);
  if (d.estado !== 'valido' || f.estado !== 'valido' || d.valor <= 0 || f.valor <= 0) return null;
  if (buyKind === 'masa' && baseKind === 'volumen') return f.valor / d.valor;
  if (buyKind === 'volumen' && baseKind === 'masa') return f.valor * d.valor;
  return null;
}

const SMALLEST: Record<string, string> = { masa: 'g', volumen: 'ml' };
// Un número, no una fecha: no lleva zona. Lo grande sin decimales («3,909»), lo chico con dos.
const qtyWords = (n: number) => n.toLocaleString('es-MX', { maximumFractionDigits: Math.abs(n) >= 100 ? 0 : 2 });
export const densityWords = (n: number) => n.toLocaleString('es-MX', { minimumFractionDigits: 2, maximumFractionDigits: 4 });

// conversionWords dice en palabras cómo se convirtió un renglón entre peso y líquido (FR-009):
// «4,300 g ÷ 1.10 = 3,909 ml». null si no aplica o falta algo.
export function conversionWords(qty: string, buy: { kind: string; toBase: string }, baseKind: string, density: string | null | undefined): string | null {
  const factor = densityFactor(buy.kind, baseKind, buy.toBase, density);
  const result = baseOfLine(qty, factor === null ? null : String(factor));
  const q = parseNumero(qty);
  if (result === null || q.estado !== 'valido') return null;
  const d = parseNumero(density ?? '');
  if (d.estado !== 'valido') return null;
  const smallest = q.valor * Number(buy.toBase);
  const op = buy.kind === 'masa' ? '÷' : '×';
  return `${qtyWords(smallest)} ${SMALLEST[buy.kind]} ${op} ${densityWords(d.valor)} = ${qtyWords(result)} ${SMALLEST[baseKind]}`;
}

const CODE_WORD: Record<string, string> = { l: 'L' };
export interface CrossConversion {
  factor: number | null; // cuánto suma una unidad comprada en la base; null sin densidad
  words: string | null; // «4,300 g ÷ 1.10 = 3,909 ml», si hay cantidad y densidad
  missing: string; // «Sin densidad para pasar kg a ml», o '' si no falta
}

// ASK_FOR_DENSITY acompaña al aviso donde no hay atajo a la densidad (sin permiso, o un artículo que
// todavía no existe): sin él, quien captura se queda sin saber cómo seguir.
export const ASK_FOR_DENSITY = 'Pídele a quien configura el almacén que capture la densidad, o elige otra unidad.';

// crossConversion es lo que un renglón de compra entre peso y líquido le dice a quien captura: su
// conversión o lo que falta para hacerla. null cuando el renglón no cruza de clase (o es pieza, o
// longitud): ahí la cuenta es la de siempre y no hay nada que decir.
export function crossConversion(
  qty: string, buy: { code: string; kind: string; toBase: string }, baseKind: string, baseCode: string, density: string | null | undefined,
): CrossConversion | null {
  const cross = (buy.kind === 'masa' && baseKind === 'volumen') || (buy.kind === 'volumen' && baseKind === 'masa');
  if (!cross) return null;
  const factor = densityFactor(buy.kind, baseKind, buy.toBase, density);
  if (factor === null) return { factor: null, words: null, missing: `Sin densidad para pasar ${CODE_WORD[buy.code] ?? buy.code} a ${baseCode}` };
  return { factor, words: conversionWords(qty, buy, baseKind, density), missing: '' };
}

const BASE_KIND: Record<string, string> = { g: 'masa', ml: 'volumen', cm: 'longitud', pieza: 'pieza' };

// lineConversion es crossConversion para las pantallas que solo tienen el código de la unidad del
// renglón y de la base (por recibir, detalle del gasto).
export function lineConversion(
  qty: string, unitCode: string | null | undefined, baseUnitCode: string | null | undefined, density: string | null | undefined,
  units: { code: string; kind: string; toBase: string }[],
): CrossConversion | null {
  const buy = units.find((u) => u.code === unitCode);
  const baseKind = BASE_KIND[baseUnitCode ?? ''];
  if (!buy || !baseKind || !baseUnitCode) return null;
  return crossConversion(qty, buy, baseKind, baseUnitCode, density);
}
