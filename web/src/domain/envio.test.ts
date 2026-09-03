import { describe, expect, test } from 'vitest';
import { envioDeLaCuenta } from './envio';

const domicilio = { serviceType: 'domicilio' as const, platformId: null };
const mostrador = { serviceType: 'mostrador' as const, platformId: null };
const plataforma = { serviceType: 'domicilio' as const, platformId: 3 };

describe('envioDeLaCuenta', () => {
  // AUSENTE ES EL DEFAULT DEL NEGOCIO; MAL ESCRITO NO ES NADA.
  //
  // Es el principio V aplicado a un campo de la pantalla: el default es para el campo vacío, nunca
  // para el que trae algo que no se puede leer. "1,000" —la coma es el separador natural en es-MX y
  // el teclado decimal de Android la ofrece— se cobraba como $20.
  test('sin capturar usa el default del negocio y no lo manda', () => {
    const e = envioDeLaCuenta(domicilio, '', 20);
    expect(e).toEqual({ aplica: true, malEscrito: false, monto: 20, paraElServidor: undefined });
  });

  test('capturado viaja tal cual', () => {
    const e = envioDeLaCuenta(domicilio, '80', 20);
    expect(e).toEqual({ aplica: true, malEscrito: false, monto: 80, paraElServidor: 80 });
  });

  test.each(['1,000', '1,5', 'ochenta', '1e5', '12kg', '-30'])(
    'mal escrito (%s) bloquea y NO manda nada', (v) => {
      const e = envioDeLaCuenta(domicilio, v, 20);
      expect(e.malEscrito).toBe(true);
      expect(e.monto).toBe(0);
      // Lo que importa: no cae al default. Mandarlo sin valor hacía que el servidor cobrara $20 de
      // un envío que el operador tecleó como mil.
      expect(e.paraElServidor).toBeUndefined();
    },
  );

  // UN ENVÍO QUE NO APLICA NO PUEDE TRABAR LA VENTA.
  //
  // El cálculo corría siempre, así que un valor mal escrito capturado en domicilio seguía apagando
  // los botones después de cambiar a mostrador o de asignar una plataforma — y ahí el campo no se
  // pinta, así que quedaba un POS mudo sin nada que corregir a la vista.
  test.each([
    ['mostrador', mostrador],
    ['plataforma', plataforma],
  ])('en %s un valor mal escrito no bloquea', (_, cuenta) => {
    const e = envioDeLaCuenta(cuenta, '1,000', 20);
    expect(e.aplica).toBe(false);
    expect(e.malEscrito).toBe(false);
    expect(e.monto).toBe(0);
  });

  // Con plataforma el envío lo cobra ella: sumarlo en pantalla ofrecía cobrar $115 de un pedido que
  // el servidor cobra en $95.
  test('con plataforma no suma envío aunque esté capturado', () => {
    expect(envioDeLaCuenta(plataforma, '80', 20).monto).toBe(0);
  });

  test('el cero explícito es envío gratis decidido, y viaja', () => {
    const e = envioDeLaCuenta(domicilio, '0', 20);
    expect(e).toEqual({ aplica: true, malEscrito: false, monto: 0, paraElServidor: 0 });
  });
});
