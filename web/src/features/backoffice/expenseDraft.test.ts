import { test, expect, describe } from 'vitest';
import { packToBase, lineAmount, splitTotals } from './expenseDraft';

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
