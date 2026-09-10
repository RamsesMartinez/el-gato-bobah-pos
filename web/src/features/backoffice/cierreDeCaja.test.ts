import { faltanPorContar, diferenciasDelCierre, type MetodoPorContar } from './cierreDeCaja';

const efectivo: MetodoPorContar = { methodId: 1, name: 'Efectivo', kind: 'efectivo', expected: '1662', autoDeclare: false };
const tarjeta: MetodoPorContar = { methodId: 2, name: 'Tarjeta débito', kind: 'tarjeta', expected: '338', autoDeclare: true };
const rappiEfe: MetodoPorContar = { methodId: 21, name: 'Rappi efectivo', kind: 'plataforma', expected: '250', autoDeclare: false };

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

// LA DIFERENCIA ANTES DE CONFIRMAR (FR-005).
//
// La tabla del cierre en vivo tenía Método / Esperado / Declarado y nada más: la única con columna
// de diferencia se pintaba en el diálogo POSTERIOR al cierre, cuando el servidor ya cerró el turno.
// El operador firmaba y después se enteraba del faltante.
describe('la diferencia del cierre, en vivo', () => {
  const efectivo: MetodoPorContar = { methodId: 1, name: 'Efectivo', kind: 'efectivo', expected: '340', autoDeclare: false };
  const tarjeta: MetodoPorContar = { methodId: 2, name: 'Tarjeta', kind: 'tarjeta', expected: '80', autoDeclare: true };

  test('un conteo exacto no reporta diferencia', () => {
    const d = diferenciasDelCierre([efectivo], { 1: 340 });
    expect(d.porMetodo[1]).toBe(0);
    expect(d.total).toBe(0);
    expect(d.completo).toBe(true);
  });

  test('contar menos de lo esperado es un faltante, con signo', () => {
    // El signo importa: un faltante manda a buscar dinero y un sobrante manda a buscar un cobro sin
    // registrar. Confundirlos hace perder el turno buscando en el lugar equivocado.
    const d = diferenciasDelCierre([efectivo], { 1: 290 });
    expect(d.porMetodo[1]).toBe(-50);
    expect(d.total).toBe(-50);
  });

  test('un método que se declara solo cuadra por construcción y no ensucia el total', () => {
    const d = diferenciasDelCierre([efectivo, tarjeta], { 1: 340 });
    expect(d.porMetodo[2]).toBe(0);
    expect(d.total).toBe(0);
  });

  test('lo que todavía no se captura no vale cero: la diferencia queda incompleta', () => {
    // Tratar el campo vacío como cero es exactamente el defecto del corte de $1,662: inventaba un
    // faltante por todo lo que el cajero no había capturado todavía.
    const d = diferenciasDelCierre([efectivo], {});
    expect(d.porMetodo[1]).toBeUndefined();
    expect(d.completo).toBe(false);
    expect(d.total).toBe(0);
  });

  test('el redondeo no arrastra centavos de la resta', () => {
    const centavos: MetodoPorContar = { ...efectivo, expected: '340.10' };
    expect(diferenciasDelCierre([centavos], { 1: 340.05 }).total).toBe(-0.05);
  });
});
