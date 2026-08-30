import { faltanPorContar, type MetodoPorContar } from './cierreDeCaja';

const efectivo: MetodoPorContar = { methodId: 1, name: 'Efectivo', expected: '1662', autoDeclare: false };
const tarjeta: MetodoPorContar = { methodId: 2, name: 'Tarjeta débito', expected: '338', autoDeclare: true };
const rappiEfe: MetodoPorContar = { methodId: 21, name: 'Rappi efectivo', expected: '250', autoDeclare: false };

// El caso real: se cerró sin capturar el efectivo y quedó un faltante de $1,662 que no existía.
test('un método que exige conteo y está en blanco impide cerrar', () => {
  const faltan = faltanPorContar([efectivo, tarjeta], {});
  expect(faltan.map((m) => m.name)).toEqual(['Efectivo']);
});

test('los que se autodeclaran no piden captura', () => {
  expect(faltanPorContar([tarjeta], {})).toEqual([]);
});

// Un cero ESCRITO es una respuesta: puede no haber efectivo. Lo que no vale es el campo vacío.
test('un cero escrito cuenta como capturado', () => {
  expect(faltanPorContar([efectivo], { '1': '0' })).toEqual([]);
});

test('espacios en blanco no cuentan como capturado', () => {
  expect(faltanPorContar([efectivo], { '1': '   ' }).length).toBe(1);
});

test('un método que no esperaba nada no obliga a capturar', () => {
  const sinMovimiento = { ...efectivo, expected: '0' };
  expect(faltanPorContar([sinMovimiento], {})).toEqual([]);
});

// Con los métodos de plataforma en efectivo hay varios que se cuentan: todos deben exigirse.
test('varios métodos de conteo físico se listan todos', () => {
  const faltan = faltanPorContar([efectivo, rappiEfe], { '1': '1662' });
  expect(faltan.map((m) => m.name)).toEqual(['Rappi efectivo']);
});
