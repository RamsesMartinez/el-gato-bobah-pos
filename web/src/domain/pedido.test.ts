import { describe, expect, it } from 'vitest';
import { armarPedido, cobraEnvio } from './pedido';
import type { TicketLine } from '../types/pos';
import type { TicketTab } from '../types/pos';

const linea = (l: Partial<TicketLine>): TicketLine => ({
  lineId: 'a', productId: 10, name: 'Alitas', unitPrice: 200, qty: 1, modifiers: [], ...l,
});

const cuenta = (c: Partial<TicketTab>): TicketTab => ({
  id: 't1', num: 1, folioName: 'Tigre', lines: [], envio: '', serviceType: 'mostrador',
  customerName: '', platformId: null, ...c,
});

describe('armarPedido', () => {
  it('manda ids y cantidades, nunca precios: el servidor los recalcula', () => {
    const body = armarPedido({
      cuenta: cuenta({}), lineas: [linea({ qty: 2 })], clientUuid: 'u1', deliveryFee: 0,
    });
    expect(body.lines).toEqual([{ productId: 10, qty: 2, notes: undefined, modifiers: [] }]);
    expect(JSON.stringify(body)).not.toContain('unitPrice');
  });

  it('sin pagos = mandado a cocina y por cobrar', () => {
    const body = armarPedido({ cuenta: cuenta({}), lineas: [linea({})], clientUuid: 'u1', deliveryFee: 0 });
    // El cuerpo NO lleva pagos, y ya no puede llevarlos: el campo se borró del tipo. Crear un
    // pedido ya cobrado era el atajo que se saltaba la cocina, y el servidor lo rechaza desde la
    // feature 005 — el parámetro se quedó muerto hasta este barrido, con este test verificando la
    // ausencia de algo que nadie podía poner.
    expect('payments' in body).toBe(false);
  });

  it('lleva el animal de la cuenta, que es el que ya vio el cliente', () => {
    const body = armarPedido({
      cuenta: cuenta({ folioName: 'Nutria' }), lineas: [linea({})], clientUuid: 'u1', deliveryFee: 0,
    });
    expect(body.folioName).toBe('Nutria');
  });

  // Un pedido de plataforma ES a domicilio y su envío lo cobra la plataforma. El servidor lo exige;
  // mandarlo ya así evita mostrar un total que el servidor no va a cobrar.
  it('una plataforma fuerza domicilio y envío en cero', () => {
    const body = armarPedido({
      cuenta: cuenta({ platformId: 3, serviceType: 'mostrador' }),
      lineas: [linea({})], clientUuid: 'u1', deliveryFee: 20,
    });
    expect(body.serviceType).toBe('domicilio');
    expect(body.deliveryFee).toBe(0);
    expect(body.deliveryPlatformId).toBe(3);
  });

  it('el envío del negocio sí viaja en un domicilio propio', () => {
    const body = armarPedido({
      cuenta: cuenta({ serviceType: 'domicilio' }), lineas: [linea({})], clientUuid: 'u1', deliveryFee: 20,
    });
    expect(body.deliveryFee).toBe(20);
    expect(body.deliveryPlatformId).toBeUndefined();
  });

  it('un nombre de cliente vacío no viaja', () => {
    const body = armarPedido({
      cuenta: cuenta({ customerName: '' }), lineas: [linea({})], clientUuid: 'u1', deliveryFee: 0,
    });
    expect(body.customerName).toBeUndefined();
  });
});

// EL DEFECTO QUE ESTO CIERRA: la pantalla ofrecía cobrar un envío que el servidor no cobra.
//
// Secuencia real y alcanzable hoy: cuenta nueva -> se marca "Domicilio" -> después se le asigna una
// plataforma. `setPlatform` no toca el tipo de servicio, y el panel del pedido esconde los botones
// de tipo cuando hay plataforma, así que el operador ya no lo ve ni lo puede corregir. La cuenta se
// queda en 'domicilio' con plataforma, la pantalla suma $20 de envío, y el servidor los fuerza a 0.
// Resultado: la hoja pinta COBRAR $115 sobre un pedido de $95, el cobro rebota por exceso, y el
// pedido queda CREADO Y SIN COBRAR.
//
// `armarPedido` ya aplicaba la regla al armar el cuerpo. Lo que faltaba era que la MISMA regla
// decidiera lo que la pantalla muestra, en vez de que cada lugar la volviera a deducir.
describe('cobraEnvio', () => {
  it('un domicilio propio sí cobra envío', () => {
    expect(cobraEnvio(cuenta({ serviceType: 'domicilio', platformId: null }))).toBe(true);
  });

  it('mostrador no cobra envío', () => {
    expect(cobraEnvio(cuenta({ serviceType: 'mostrador', platformId: null }))).toBe(false);
  });

  it('con plataforma NO cobra envío, aunque la cuenta diga domicilio', () => {
    // Es el caso caro: el reparto lo cobra la plataforma, no el negocio.
    expect(cobraEnvio(cuenta({ serviceType: 'domicilio', platformId: 3 }))).toBe(false);
  });

  it('lo que decide la pantalla y lo que viaja al servidor no pueden divergir', () => {
    // La garantía de que son la misma regla: si alguien cambia una, este test truena.
    for (const c of [
      cuenta({ serviceType: 'domicilio', platformId: null }),
      cuenta({ serviceType: 'domicilio', platformId: 3 }),
      cuenta({ serviceType: 'mostrador', platformId: null }),
      cuenta({ serviceType: 'mostrador', platformId: 3 }),
    ]) {
      const body = armarPedido({ cuenta: c, lineas: [], clientUuid: 'u', deliveryFee: 20 });
      expect(Number(body.deliveryFee) > 0).toBe(cobraEnvio(c));
    }
  });
});
