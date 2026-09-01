import { describe, expect, it } from 'vitest';
import { armarPedido } from './armarPedido';
import type { TicketLine } from '../../types/pos';
import type { TicketTab } from '../../stores/ticket';

const linea = (l: Partial<TicketLine>): TicketLine => ({
  lineId: 'a', productId: 10, name: 'Alitas', unitPrice: 200, qty: 1, modifiers: [], ...l,
});

const cuenta = (c: Partial<TicketTab>): TicketTab => ({
  id: 't1', num: 1, folioName: 'Tigre', lines: [], serviceType: 'mostrador',
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
    expect(body.payments).toBeUndefined();
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
