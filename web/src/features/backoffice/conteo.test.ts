import { armarConteo, leerPiezas, type Denominacion } from './conteo';

// El catálogo llega del servidor con los valores como STRING de `numeric(10,2)`, de mayor a menor.
const catalogo: Denominacion[] = [
  { id: 11, value: '1000.00', isCoin: false },
  { id: 6, value: '50.00', isCoin: false },
  { id: 5, value: '10.00', isCoin: true },
  { id: 1, value: '0.50', isCoin: true },
];

// LOS MISMOS FIXTURES QUE `conteo_test.go`. No es duplicación por descuido: si la pantalla y el
// servidor no cuentan con los mismos casos, la pantalla puede mostrar $210 y el servidor guardar
// otra cifra, y el operador firma un arqueo que no vio.
describe('el total del conteo', () => {
  test('el cajón del spec: 6 monedas de $10 y 3 billetes de $50 suman $210', () => {
    expect(armarConteo(catalogo, { 5: '6', 6: '3' }).total).toBe(210);
  });

  test('7 monedas de 50 centavos son $3.50 y no $3', () => {
    // Truncar el medio peso es exactamente el error que esta feature viene a quitar.
    expect(armarConteo(catalogo, { 1: '7' }).total).toBe(3.5);
  });

  test('un cajón vacío suma cero y no manda renglones', () => {
    const c = armarConteo(catalogo, {});
    expect(c.total).toBe(0);
    expect(c.renglones).toEqual([]);
  });

  test('un cero escrito no aporta ni estorba', () => {
    const c = armarConteo(catalogo, { 11: '0', 6: '2' });
    expect(c.total).toBe(100);
    // FR-009: el cero es válido de entrada y simplemente no genera renglón — "no hay" y "no se
    // contó" se guardan igual, que es lo que decidió el esquema.
    expect(c.renglones).toEqual([{ denominationId: 6, pieces: 2 }]);
  });

  test('los renglones salen en el orden del catálogo, de mayor a menor', () => {
    const c = armarConteo(catalogo, { 1: '4', 11: '1', 5: '2' });
    expect(c.renglones.map((r) => r.denominationId)).toEqual([11, 5, 1]);
  });

  test('una captura de una denominación que ya no está en el catálogo no viaja', () => {
    // Pasa al cambiar de moneda o al retirarse una denominación: el estado de la pantalla conserva
    // el id viejo. Mandarlo haría que el servidor rechace el conteo entero por un id que el
    // operador no puede ver ni corregir.
    const c = armarConteo(catalogo, { 6: '1', 999: '3' });
    expect(c.renglones).toEqual([{ denominationId: 6, pieces: 1 }]);
    expect(c.total).toBe(50);
  });
});

// El campo tecleado es el control PRINCIPAL de cada denominación, así que sus bordes son el camino
// caliente, no un caso raro.
describe('leer el campo de piezas', () => {
  test('vacío no es cero capturado', () => {
    // Borrar el campo para reescribirlo no puede significar "conté cero": es "no lo he escrito".
    expect(leerPiezas('')).toEqual({ estado: 'vacio' });
    expect(leerPiezas('   ')).toEqual({ estado: 'vacio' });
  });

  test('un cero escrito sí es una respuesta', () => {
    expect(leerPiezas('0')).toEqual({ estado: 'valido', piezas: 0 });
  });

  test('40 piezas se leen como 40', () => {
    expect(leerPiezas('40')).toEqual({ estado: 'valido', piezas: 40 });
  });

  test('la coma de millar no se convierte en otro número', () => {
    // `parseFloat('1,000')` devuelve 1 y ES finito, así que la guarda ingenua no lo atrapa. En
    // piezas eso es declarar 1 billete donde hay mil.
    expect(leerPiezas('1,000')).toEqual({ estado: 'invalido', motivo: 'formato' });
  });

  test('la basura no cae a cero en silencio', () => {
    expect(leerPiezas('4o')).toEqual({ estado: 'invalido', motivo: 'formato' });
    expect(leerPiezas('40 piezas')).toEqual({ estado: 'invalido', motivo: 'formato' });
  });

  test('piezas negativas se rechazan', () => {
    expect(leerPiezas('-1')).toEqual({ estado: 'invalido', motivo: 'negativo' });
  });

  test('media moneda no existe', () => {
    // Este chequeo no lo hace `numeros.ts` y por eso vive aquí: ahí se leen dinero y stock, que sí
    // llevan decimales. Una pieza fraccionaria mete un total que el cajón no puede formar.
    expect(leerPiezas('1.5')).toEqual({ estado: 'invalido', motivo: 'fraccion' });
    expect(leerPiezas('2.0')).toEqual({ estado: 'valido', piezas: 2 });
  });
});

describe('una captura inválida', () => {
  test('se nombra y no suma cero en silencio', () => {
    const c = armarConteo(catalogo, { 6: '2', 5: '1,000' });
    expect(c.invalidas).toEqual([5]);
    // El total NO incluye la denominación mal capturada, y quien llama apaga el botón mientras
    // `invalidas` traiga algo: un total que se ve completo y no lo está es peor que un error.
    expect(c.total).toBe(100);
  });

  test('no manda renglón', () => {
    const c = armarConteo(catalogo, { 5: '4o' });
    expect(c.renglones).toEqual([]);
    expect(c.invalidas).toEqual([5]);
  });
});
