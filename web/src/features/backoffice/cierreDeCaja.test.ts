import {
  faltanPorContar, diferenciasDelCierre, faltaContarElCajon, diferenciaDelCajon,
  type MetodoPorContar, type ArqueoDelCajon,
} from './cierreDeCaja';

const efectivo: MetodoPorContar = { methodId: 1, name: 'Efectivo', kind: 'efectivo', expected: '1662', autoDeclare: false, requiresEntry: true };
const tarjeta: MetodoPorContar = { methodId: 2, name: 'Tarjeta débito', kind: 'tarjeta', expected: '338', autoDeclare: true, requiresEntry: false };
const rappiEfe: MetodoPorContar = { methodId: 21, name: 'Rappi efectivo', kind: 'plataforma', expected: '250', autoDeclare: false, requiresEntry: true };

// El caso real: se cerró sin capturar el efectivo y quedó un faltante de $1,662 que no existía.
test('un método que exige conteo y está en blanco impide cerrar', () => {
  const faltan = faltanPorContar([efectivo, tarjeta], {});
  expect(faltan.map((m) => m.name)).toEqual(['Efectivo']);
});

test('los que se autodeclaran no piden captura', () => {
  expect(faltanPorContar([tarjeta], {})).toEqual([]);
});

// EL SERVIDOR DECIDE QUÉ FALTA, no la pantalla mirando si el esperado es cero.
//
// Es el hallazgo de la revisión de arquitectura de la spec 015: con el arqueo ciego el esperado
// viaja en NULL, `Number(null)` es 0, y la regla vieja concluía que ningún método esperaba dinero.
// El botón de cerrar quedaba habilitado con la pantalla en blanco — el faltante inventado de $1,662
// por la puerta de atrás.
test('con el esperado en null (arqueo ciego) sigue exigiendo lo que falta', () => {
  const ciego: MetodoPorContar = { ...efectivo, expected: null };
  expect(faltanPorContar([ciego], {}).map((m) => m.name)).toEqual(['Efectivo']);
});

test('un método del cajón no pide cifra propia: su dinero se declara contándolo', () => {
  const delCajon: MetodoPorContar = { ...efectivo, requiresEntry: false };
  expect(faltanPorContar([delCajon], {})).toEqual([]);
});

// Un cero ESCRITO es una respuesta: puede no haber efectivo. Lo que no vale es el campo vacío.
test('un cero escrito cuenta como capturado', () => {
  expect(faltanPorContar([efectivo], { '1': '0' })).toEqual([]);
});

test('espacios en blanco no cuentan como capturado', () => {
  expect(faltanPorContar([efectivo], { '1': '   ' }).length).toBe(1);
});

test('un método que no esperaba nada no obliga a capturar', () => {
  // El servidor ya lo resolvió: si no esperaba dinero, no exige captura.
  const sinMovimiento = { ...efectivo, expected: '0', requiresEntry: false };
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
  // DESDE LA SPEC 015 ESTA ARITMÉTICA ES DE LOS MÉTODOS QUE **NO** ESTÁN EN EL CAJÓN.
  //
  // El efectivo tenía aquí su propia diferencia y ya no: su dinero se arquea una sola vez, con el
  // cajón completo, y esa cuenta vive en `diferenciaDelCajon`. Los casos de abajo son los mismos de
  // antes, movidos al método que sí declara una cifra propia — una transferencia.
  const transfer: MetodoPorContar = { methodId: 3, name: 'Transferencia', kind: 'transferencia', expected: '340', autoDeclare: false, requiresEntry: true };
  const tarjeta: MetodoPorContar = { methodId: 2, name: 'Tarjeta', kind: 'tarjeta', expected: '80', autoDeclare: true, requiresEntry: false };
  const delCajon: MetodoPorContar = { methodId: 1, name: 'Efectivo', kind: 'efectivo', expected: '340', autoDeclare: false, requiresEntry: false };

  test('una cifra exacta no reporta diferencia', () => {
    const d = diferenciasDelCierre([transfer], { 3: 340 });
    expect(d.porMetodo[3]).toBe(0);
    expect(d.total).toBe(0);
    expect(d.completo).toBe(true);
  });

  test('declarar menos de lo esperado es un faltante, con signo', () => {
    // El signo importa: un faltante manda a buscar dinero y un sobrante manda a buscar un cobro sin
    // registrar. Confundirlos hace perder el turno buscando en el lugar equivocado.
    const d = diferenciasDelCierre([transfer], { 3: 290 });
    expect(d.porMetodo[3]).toBe(-50);
    expect(d.total).toBe(-50);
  });

  test('un método que se declara solo cuadra por construcción y no ensucia el total', () => {
    const d = diferenciasDelCierre([transfer, tarjeta], { 3: 340 });
    expect(d.porMetodo[2]).toBe(0);
    expect(d.total).toBe(0);
  });

  test('un método del cajón no aporta diferencia propia', () => {
    // Es el defecto que la 015 vino a cerrar: con una diferencia por método, un faltante del cajón
    // y un sobrante de otro renglón se cancelaban y el corte parecía cuadrado.
    const d = diferenciasDelCierre([delCajon], { 1: 290 });
    expect(d.porMetodo[1]).toBeUndefined();
    expect(d.total).toBe(0);
  });

  test('lo que todavía no se captura no vale cero: la diferencia queda incompleta', () => {
    // Tratar el campo vacío como cero es exactamente el defecto del corte de $1,662: inventaba un
    // faltante por todo lo que el cajero no había capturado todavía.
    const d = diferenciasDelCierre([transfer], {});
    expect(d.porMetodo[3]).toBeUndefined();
    expect(d.completo).toBe(false);
    expect(d.total).toBe(0);
  });

  test('el redondeo no arrastra centavos de la resta', () => {
    const centavos: MetodoPorContar = { ...transfer, expected: '340.10' };
    expect(diferenciasDelCierre([centavos], { 3: 340.05 }).total).toBe(-0.05);
  });
});


// EL ARQUEO DEL CAJÓN, EN VIVO Y ANTES DE FIRMAR (spec 015).
//
// El cajón es un solo montón de billetes y su diferencia es una sola. Lo que esto defiende es que
// la pantalla no vuelva a repartirla entre métodos —de ahí salió el turno con «Efectivo» en $0.00 y
// «Didi efectivo» en −$64.80— y que con el arqueo ciego no muestre nada que no deba.
describe('el arqueo del cajón', () => {
  const cajon: ArqueoDelCajon = {
    expected: '800', counted: null, difference: null,
    methodIds: [1, 8], requiresCount: true,
  };

  test('sin contar, el cierre no procede', () => {
    expect(faltaContarElCajon(cajon, null)).toBe(true);
  });

  test('con el conteo hecho, ya no falta', () => {
    expect(faltaContarElCajon(cajon, { counts: [], total: 800 })).toBe(false);
  });

  test('un cajón que no espera nada no obliga a contar', () => {
    expect(faltaContarElCajon({ ...cajon, expected: '0', requiresCount: false }, null)).toBe(false);
  });

  test('sin cajón no hay nada que contar', () => {
    expect(faltaContarElCajon(null, null)).toBe(false);
  });

  test('contar menos de lo esperado es UN faltante, del cajón', () => {
    const d = diferenciaDelCajon(cajon, { counts: [], total: 750 });
    expect(d).toBe(-50);
  });

  test('con el arqueo ciego no se calcula diferencia: el esperado no viaja', () => {
    // Y no se inventa un cero, que se leería como "cuadra".
    expect(diferenciaDelCajon({ ...cajon, expected: null }, { counts: [], total: 750 })).toBeUndefined();
  });

  test('sin conteo todavía no hay diferencia', () => {
    expect(diferenciaDelCajon(cajon, null)).toBeUndefined();
  });
});

// CON EL ARQUEO CIEGO NO HAY DIFERENCIA QUE MOSTRAR, Y CERO NO ES "NO HAY".
//
// `Number(null)` es 0, así que restar contra un esperado que no viajó daba la cifra capturada
// entera como diferencia: el operador teclea $1,200 de tarjeta y la pantalla le pinta
// «Diferencia $1,200.00» en verde, que es un sobrante inventado — y justo antes de confirmar, que
// es cuando decide si vuelve a contar. Es el mismo `Number(null) === 0` que ya se arregló en
// `faltanPorContar` y que seguía vivo aquí.
test('con el esperado en null la diferencia no existe, no es cero', () => {
  const { porMetodo, total, completo } = diferenciasDelCierre(
    [{ methodId: 2, name: 'Tarjeta débito', kind: 'tarjeta', expected: null, autoDeclare: false, requiresEntry: true }],
    { 2: 1200 },
  );
  expect(porMetodo[2]).toBeUndefined();
  expect(total).toBe(0);
  // Y la captura sí se registró: lo que falta es contra qué comparar, no la cifra.
  expect(completo).toBe(true);
});

// Y LA SEÑAL DE "FALTA CAPTURAR" SOBREVIVE AL ARQUEO CIEGO.
//
// El borde del arreglo de arriba: si "sin esperado no hay diferencia" se evaluara ANTES de mirar
// si el método tiene captura, el cierre a ciegas se reportaría completo con los campos vacíos —
// que es la misma puerta de atrás, con otra llave.
test('con el arqueo ciego, un método sin capturar sigue dejando el cierre incompleto', () => {
  const { completo } = diferenciasDelCierre(
    [{ methodId: 2, name: 'Tarjeta débito', kind: 'tarjeta', expected: null, autoDeclare: false, requiresEntry: true }],
    {},
  );
  expect(completo).toBe(false);
});
