import { test, expect, describe } from 'vitest';
import { packToBase, lineAmount, splitTotals, purchaseMoney, blankLine, baseOfLine, densityFactor, conversionWords, crossConversion, lineConversion } from './expenseDraft';

// Unidades como las siembra la migración 0010.
const units = [
  { code: 'g', toBase: '1' },
  { code: 'kg', toBase: '1000' },
  { code: 'ml', toBase: '1' },
  { code: 'l', toBase: '1000' },
  { code: 'pieza', toBase: '1' },
];

// El contenido de una unidad de compra se guarda en unidad BASE del almacén. Pasar el número tal
// como lo imprime el documento mete "2.26 gramos" de queso en vez de 2260: el error no se ve en
// la pantalla, se ve semanas después en un inventario imposible.
describe('packToBase', () => {
  test.each([
    // Casos de los documentos reales de docs/tickets/
    ['2.26K MOZZAR', '2.26', 'kg', '2260'],
    ['MM 2K FRESA', '2', 'kg', '2000'],
    ['6/186GR CHAM', '1116', 'g', '1116'],
    ['Harina … 432 g', '432', 'g', '432'],
    ['Suavizante … 5.1 l', '5.1', 'l', '5100'],
    ['Pasta dental 2x67 ml', '67', 'ml', '67'],
    ['Servilletas 200 pzas', '200', 'pzas', '200'],
  ])('%s → %s %s = %s base', (_desc, qty, unit, want) => {
    expect(packToBase(qty, unit, units)).toBe(want);
  });

  test('unidad no reconocida deja el campo vacío en vez de arriesgar la escala', () => {
    // "270 hojas dobles" no es una unidad de almacén: mejor que el operador lo llene.
    expect(packToBase('270', 'hojas', units)).toBe('');
    expect(packToBase('200', 'HD', units)).toBe('');
    expect(packToBase('1', 'rollos', units)).toBe('');
  });

  test('sin cantidad, cero o basura no inventa nada', () => {
    expect(packToBase('', 'kg', units)).toBe('');
    expect(packToBase('0', 'kg', units)).toBe('');
    expect(packToBase('-2', 'kg', units)).toBe('');
    expect(packToBase('abc', 'kg', units)).toBe('');
  });

  test('tolera cómo escriben la unidad los documentos', () => {
    for (const u of ['KG', ' kg ', 'Kgs', 'kilos', 'K']) {
      expect(packToBase('1', u, units)).toBe('1000');
    }
    for (const u of ['GR', 'grs', 'Gramos']) {
      expect(packToBase('500', u, units)).toBe('500');
    }
  });

  test('una unidad ausente del catálogo no se asume', () => {
    // Si la empresa no tiene 'l' dado de alta, no se puede convertir litros.
    expect(packToBase('5.1', 'l', [{ code: 'g', toBase: '1' }])).toBe('');
  });
});

// Hay documentos que solo publican el precio "c/u" y nunca el importe del renglón. Sin derivarlo,
// esas líneas se guardarían en 0 y el gasto no cuadraría contra su propio total.
describe('lineAmount', () => {
  test('el importe impreso manda', () => {
    expect(lineAmount('51.00', '17.00', '3')).toBe('51.00');
  });
  test('solo unitario: multiplica por la cantidad', () => {
    // Pedido real de Walmart: 3 servitoallas a $17.00 c/u, sin total de renglón impreso.
    expect(lineAmount('', '17.00', '3')).toBe('51.00');
    expect(lineAmount('', '8.07', '3')).toBe('24.21');
  });
  test('sin cantidad es una unidad', () => {
    expect(lineAmount('', '22.00', '')).toBe('22.00');
    expect(lineAmount('', '22.00', '0')).toBe('22.00');
  });
  test('sin importe ni unitario no inventa un número', () => {
    expect(lineAmount('', '', '3')).toBe('');
  });
});

// Lo que es del local y lo que es de la casa son dos totales distintos. Sumarlos juntos infla los
// gastos del negocio con el shampoo del ticket del Sam's.
describe('splitTotals', () => {
  test('separa los dos totales', () => {
    const { local, personal } = splitTotals([
      { amount: '100.50', personal: false },
      { amount: '200.00', personal: true },
      { amount: '50.25', personal: false },
    ]);
    expect(local).toBe(150.75);
    expect(personal).toBe(200);
  });

  test('un importe ilegible no cuenta como cero silencioso ni rompe la suma', () => {
    const { local } = splitTotals([{ amount: '', personal: false }, { amount: '10', personal: false }]);
    expect(local).toBe(10);
  });

  test('sin líneas de casa el total personal es 0', () => {
    expect(splitTotals([{ amount: '10', personal: false }])).toEqual({ local: 10, personal: 0 });
  });

  test('redondea a centavos: sumar flotantes deja colas que descuadran el documento', () => {
    // 0.1 + 0.2 = 0.30000000000000004 en JS; contra un total de 0.30 nunca cuadraría.
    expect(splitTotals([{ amount: '0.1', personal: false }, { amount: '0.2', personal: false }]).local).toBe(0.3);
  });
});

// EL DEFECTO QUE ESTO CIERRA: `parseFloat('1,000')` devuelve 1, y es un número finito, así que la
// guarda de `Number.isFinite` que había no lo atrapaba. El operador teclea la coma de millar por
// costumbre y el contenido del empaque entra mil veces más chico — el stock queda corrupto y nadie
// se entera hasta el inventario.
test('una cantidad con coma de millar se rechaza, no se lee como 1', () => {
  const unidades = [{ code: 'kg', toBase: '1000' }];
  expect(packToBase('1,000', 'kg', unidades)).toBe('');
  // Y la misma cantidad bien escrita sí pasa: la regla rechaza el formato, no el número.
  expect(packToBase('1000', 'kg', unidades)).toBe('1000000');
});

// `parseFloat('12kg')` devuelve 12: la unidad tecleada dentro del campo se descartaba en silencio.
test('un número con la unidad pegada se rechaza en vez de tirar la cola', () => {
  expect(packToBase('12kg', 'kg', [{ code: 'kg', toBase: '1000' }])).toBe('');
});

// El importe de un renglón mal escrito NO se cuenta en el total del documento: sumarlo como otra
// cifra dejaría el cuadre contra el total impreso fallando sin decir por cuál renglón.
test('un importe con coma no se suma al total del documento', () => {
  const { local } = splitTotals([{ amount: '1,000', personal: false }, { amount: '250', personal: false }]);
  expect(local).toBe(250);
});

describe('purchaseMoney (FR-004a)', () => {
  const goods = (amount: string, gift = false) => ({ amount, goods: true, gift });
  test('un renglón sin importe se cuenta como faltante; un regalo en 0 no', () => {
    const m = purchaseMoney([goods(''), goods('0'), goods('0', true)], 100);
    expect(m.missingAmount).toBe(2);
  });
  // El servidor rechaza un renglón en 0 sea o no del almacén (FR-004a); si la pantalla deja
  // guardarlo, quien opera topa con un error al tocar Guardar.
  test('un renglón que no es del almacén también lleva importe; excluido en 0 no falta', () => {
    const m = purchaseMoney([{ amount: '0', goods: false, gift: false }, { amount: '0', goods: false, gift: false, excluded: true }], 100);
    expect(m.missingAmount).toBe(1);
  });
  test('renglones sobre el total se marcan; lo que falta queda como «otros»', () => {
    expect(purchaseMoney([goods('730'), goods('410.01')], 1140).over).toBe(true);
    const m = purchaseMoney([goods('190'), goods('540')], 1140);
    expect(m).toMatchObject({ linesSum: 730, over: false, others: 410 });
  });
  test('lo de la casa no suma al gasto', () => {
    expect(purchaseMoney([goods('100'), { amount: '50', goods: false, gift: false, excluded: true }], 100).over).toBe(false);
  });
  test('sin centavos fantasma al sumar', () => {
    expect(purchaseMoney([goods('0.1'), goods('0.2')], 0.3).over).toBe(false);
  });
});

// La captura con mercancía abre con un renglón vacío; guardar sin tocarlo mandaba un renglón en 0
// que la base rechazaba. Uno que no dice nada no es un renglón: no se manda.
describe('blankLine', () => {
  const blank = { description: '', rawName: '', itemId: null, amount: '' };
  test('el renglón que la pantalla agrega y nadie llenó', () => {
    expect(blankLine(blank)).toBe(true);
    expect(blankLine({ ...blank, amount: '0' })).toBe(true);
  });
  test('cualquier dato lo vuelve un renglón', () => {
    expect(blankLine({ ...blank, description: 'Fresas' })).toBe(false);
    expect(blankLine({ ...blank, rawName: 'FRESA KG' })).toBe(false);
    expect(blankLine({ ...blank, itemId: 7 })).toBe(false);
    expect(blankLine({ ...blank, amount: '12' })).toBe(false);
  });
  // Un importe escrito mal es un dato: descartarlo mandaría ese dinero callado a «otros».
  test('un importe escrito mal no vuelve vacío al renglón', () => {
    expect(blankLine({ ...blank, amount: '1,500' })).toBe(false);
    expect(blankLine({ ...blank, amount: '-50' })).toBe(false);
    expect(blankLine({ ...blank, amount: '12.' })).toBe(false);
  });
});

describe('baseOfLine', () => {
  test('cantidad por lo que trae el empaque', () => {
    expect(baseOfLine('3', '12000')).toBe(36000);
    expect(baseOfLine('1.5', '12000')).toBe(18000);
  });
  test('sin empaque o sin cantidad no inventa un cero', () => {
    expect(baseOfLine('3', null)).toBeNull();
    expect(baseOfLine('', '12000')).toBeNull();
    expect(baseOfLine('0', '12000')).toBeNull();
  });
});

// Spec 058 (FR-007, FR-009): la pantalla convierte entre peso y líquido con la MISMA regla que
// domain.BaseQty. Si no, mostraría 4.3 kg como «no compatible» mientras el servidor lo acepta, o
// un resultado distinto del que el almacén va a sumar.
describe('densityFactor', () => {
  test('masa → volumen divide entre la densidad: 4.3 kg con 1.1 son 3,909.0909 ml', () => {
    expect(baseOfLine('4.3', String(densityFactor('masa', 'volumen', '1000', '1.1')))).toBe(3909.0909);
  });
  test('volumen → masa multiplica: 2 L con 0.92 son 1,840 g', () => {
    expect(baseOfLine('2', String(densityFactor('volumen', 'masa', '1000', '0.92')))).toBe(1840);
  });
  test.each([
    ['sin densidad', 'masa', 'volumen', '1000', null],
    ['densidad vacía', 'masa', 'volumen', '1000', ''],
    ['densidad cero', 'masa', 'volumen', '1000', '0'],
    ['longitud nunca', 'longitud', 'masa', '100', '1.1'],
    ['hacia longitud nunca', 'masa', 'longitud', '1000', '1.1'],
    ['pieza no usa densidad', 'pieza', 'volumen', '1', '1.1'],
    ['misma clase no es asunto de la densidad', 'masa', 'masa', '1000', '1.1'],
  ])('%s → null', (_n, buy, base, toBase, d) => {
    expect(densityFactor(buy, base, toBase, d)).toBeNull();
  });
});

describe('conversionWords', () => {
  test('dice la cuenta con la densidad y el resultado en la unidad base', () => {
    expect(conversionWords('4.3', { kind: 'masa', toBase: '1000' }, 'volumen', '1.1')).toBe('4,300 g ÷ 1.10 = 3,909 ml');
    expect(conversionWords('2', { kind: 'volumen', toBase: '1000' }, 'masa', '0.92')).toBe('2,000 ml × 0.92 = 1,840 g');
  });
  test('sin densidad o con una cantidad que no es número no dice nada', () => {
    expect(conversionWords('4.3', { kind: 'masa', toBase: '1000' }, 'volumen', null)).toBeNull();
    expect(conversionWords('', { kind: 'masa', toBase: '1000' }, 'volumen', '1.1')).toBeNull();
  });
});

// Un mismo cálculo para las pantallas que tienen la unidad (captura) y las que solo tienen su código
// (por recibir, detalle). Fuera de peso ↔ líquido no dice nada: ahí la cuenta es la de siempre.
describe('crossConversion / lineConversion', () => {
  const kg = { code: 'kg', kind: 'masa', toBase: '1000' };
  const l = { code: 'l', kind: 'volumen', toBase: '1000' };
  const all = [kg, l, { code: 'g', kind: 'masa', toBase: '1' }, { code: 'ml', kind: 'volumen', toBase: '1' }, { code: 'pieza', kind: 'pieza', toBase: '1' }];
  test('con densidad: el factor y la cuenta', () => {
    expect(crossConversion('4.3', kg, 'volumen', 'ml', '1.1')).toMatchObject({ missing: '', words: '4,300 g ÷ 1.10 = 3,909 ml' });
    expect(crossConversion('4.3', kg, 'volumen', 'ml', '1.1')!.factor).toBeCloseTo(909.0909, 4);
  });
  test('sin densidad dice qué falta, sin factor', () => {
    expect(crossConversion('2', l, 'masa', 'g', null)).toEqual({ factor: null, words: null, missing: 'Sin densidad para pasar L a g' });
  });
  test('misma clase o pieza: no aplica', () => {
    expect(crossConversion('2', kg, 'masa', 'g', '1.1')).toBeNull();
    expect(crossConversion('2', { code: 'pieza', kind: 'pieza', toBase: '1' }, 'volumen', 'ml', '1.1')).toBeNull();
  });
  test('por el código de la unidad del renglón', () => {
    expect(lineConversion('2', 'l', 'g', '0.92', all)?.words).toBe('2,000 ml × 0.92 = 1,840 g');
    expect(lineConversion('2', 'pieza', 'g', '0.92', all)).toBeNull();
    expect(lineConversion('2', null, 'g', '0.92', all)).toBeNull();
  });
});
