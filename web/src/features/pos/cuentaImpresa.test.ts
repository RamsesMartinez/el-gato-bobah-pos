import { describe, expect, test } from 'vitest';
import { buildReceiptHtml } from '../../utils/printReceipt';
import type { DraftView, OrderView } from '../../types/pos';
import { armarVista } from './cuentaEnPantalla';
import { cuentaImpresa } from './cuentaImpresa';

// LA CUENTA IMPRESA (spec 012) NO SE PIERDE CON LA PUERTA ÚNICA (spec 030).
//
// Antes salía de la hoja de cobro sobre una cuenta sin confirmar. Con «Enviar y cobrar» la hoja ya
// solo se abre sobre un pedido, así que el papel de una cuenta que no se ha mandado tiene que salir
// del ticket: con el nombre amarrado y SIN número (D-2), con lo que la cuenta tiene.

const ahora = new Date('2026-10-08T20:00:00Z');
const negocio = {
  businessName: 'El Gato Bobah', address: '', phone: '', headerNote: '', footerNote: '¡Gracias!',
  logoDataUri: '', timezone: 'America/Mexico_City',
};
const cab = { serviceType: 'mostrador' as const, platformId: null, platformOrderRef: '', customerName: '' };

function draft(over: Partial<DraftView> = {}): DraftView {
  return {
    id: 'd-1', orderId: null, folioName: 'Levkoy', status: 'capturando', headerVersion: 1, updatedAt: '', createdAt: '',
    openedBy: 'Ana', serviceType: 'mostrador', customerName: null, platformId: null, platformOrderRef: null,
    deliveryFee: '0.00', discount: null, subtotal: '74.00', discountTotal: '0.00', total: '74.00', unavailable: [],
    lines: [
      { id: 'l-1', version: 1, productId: 1, productName: 'Coca Cola 355ml', qty: '1', unitPrice: '29.00', modifiers: [], notes: '', lineTotal: '29.00', available: true },
      { id: 'l-2', version: 1, productId: 2, productName: 'Chai Miel', qty: '1', unitPrice: '45.00', modifiers: [{ optionId: 7, name: 'Perlas', qty: 1, priceDelta: '0.00' }], notes: '', lineTotal: '45.00', available: true },
    ],
    ...over,
  };
}

const PEDIDO = {
  id: 9, number: 17, folioName: 'Khao Manee', status: 'abierta', serviceType: 'mostrador', deliveryPlatformId: null,
  platformOrderRef: null, customerName: null, subtotal: '130.00', discount: '0.00', deliveryFee: '0.00', total: '130.00',
  currency: 'MXN', paid: false, outstanding: '130.00', openedAt: '2026-10-08T19:00:00Z', payments: [],
  lines: [{ id: 1, productName: 'Kit Kat', quantity: '1', unitPrice: '85.00', lineTotal: '85.00', delivered: '0', cancelled: false },
    { id: 2, productName: 'Chai Miel', quantity: '1', unitPrice: '45.00', lineTotal: '45.00', delivered: '0', cancelled: false },
    { id: 3, productName: 'Malteada', quantity: '1', unitPrice: '60.00', lineTotal: '60.00', delivered: '0', cancelled: true }],
} as OrderView;

describe('una cuenta que no se ha mandado a cocina', () => {
  const papel = () => {
    const c = cuentaImpresa(armarVista({ draft: draft(), pendientes: [], guardando: false, cabeceraNueva: cab }), ahora)!;
    return { c, html: buildReceiptHtml(c.order, negocio, { preCuenta: c.preCuenta }) };
  };

  test('sale como pre-cuenta, con el nombre amarrado y sin número', () => {
    const { c, html } = papel();
    expect(c.preCuenta).toBe(true);
    expect(html).toContain('** PRE-CUENTA **');
    expect(html).toContain('Levkoy');
    expect(html).not.toContain('Pedido #');
    expect(html).not.toContain('POR COBRAR');
  });

  test('lleva sus renglones y el total que se va a cobrar', () => {
    const { c, html } = papel();
    expect(html).toContain('Coca Cola 355ml');
    expect(html).toContain('Chai Miel');
    expect(c.order.total).toBe('74.00');
  });

  // El total del papel y el que se cobra son el mismo, con descuento y envío.
  test('con descuento y envío las tres cifras cierran', () => {
    const d = draft({ serviceType: 'domicilio', subtotal: '74.00', discountTotal: '10.00', deliveryFee: '20.00', total: '84.00' });
    const { order } = cuentaImpresa(armarVista({ draft: d, pendientes: [], guardando: false, cabeceraNueva: cab }), ahora)!;
    expect(order.subtotal).toBe('74.00');
    expect(order.discount).toBe('10.00');
    expect(order.deliveryFee).toBe('20.00');
    expect(order.total).toBe('84.00');
  });

  test('una cuenta sin productos no tiene papel', () => {
    expect(cuentaImpresa(armarVista({ draft: draft({ lines: [] }), pendientes: [], guardando: false, cabeceraNueva: cab }), ahora)).toBeNull();
  });
});

describe('un pedido con algo nuevo', () => {
  test('lleva lo enviado y lo nuevo, con su número y la suma de los dos totales', () => {
    const nuevo = draft({ orderId: 9, folioName: null, lines: [draft().lines![0]], subtotal: '29.00', total: '29.00' });
    const c = cuentaImpresa(armarVista({ order: PEDIDO, draft: nuevo, pendientes: [], guardando: false, cabeceraNueva: cab }), ahora)!;
    const html = buildReceiptHtml(c.order, negocio, { preCuenta: c.preCuenta });
    expect(c.preCuenta).toBe(false);
    expect(html).toContain('Pedido #17');
    for (const p of ['Kit Kat', 'Chai Miel', 'Coca Cola 355ml']) expect(html).toContain(p);
    // Lo cancelado no se cobra: no va en el papel.
    expect(html).not.toContain('Malteada');
    expect(c.order.total).toBe('159.00');
  });
});
