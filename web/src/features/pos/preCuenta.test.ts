import { preCuentaDeLaCuenta } from './preCuenta';
import { buildReceiptHtml } from '../../utils/printReceipt';
import type { TicketLine } from '../../types/pos';

const ahora = new Date('2026-09-05T20:00:00Z');

const linea = (over: Partial<TicketLine> = {}): TicketLine => ({
  lineId: 'a', productId: 1, name: 'Alitas', unitPrice: 95, qty: 2, modifiers: [], ...over,
});

const negocio = {
  businessName: 'El Gato Bobah', address: '', phone: '', headerNote: '', footerNote: '¡Gracias!',
  logoDataUri: '', timezone: 'America/Mexico_City',
};

function papel(over: Parameters<typeof preCuentaDeLaCuenta>[0] extends infer T ? Partial<T> : never = {}) {
  const cuenta = preCuentaDeLaCuenta({
    folioName: 'Chartreux', serviceType: 'mostrador', customerName: '',
    lineas: [linea()], envio: 0, total: 190, ...over,
  }, ahora);
  return buildReceiptHtml(cuenta, negocio, { preCuenta: true });
}

// EL PAPEL DE UNA CUENTA NO PUEDE PASAR POR UN COMPROBANTE DE VENTA.
//
// Sale de la misma impresora que los tickets. Dos papeles indistinguibles del mismo pedido pueden
// circular como dos ventas, y éste es peor: ni siquiera corresponde a una venta que ocurrió.
test('lleva la marca de pre-cuenta', () => {
  expect(papel()).toContain('** PRE-CUENTA **');
});

// El número lo asigna el servidor al confirmar. Inventar uno sería peor que no poner ninguno: el
// operador se lo diría al cliente y no coincidiría con su ticket.
test('no lleva número de pedido', () => {
  expect(papel()).not.toContain('Pedido #');
});

// El nombre SÍ va: la pantalla lo propone de la misma bolsa de la que el servidor reparte.
test('lleva el nombre que la pantalla propone', () => {
  expect(papel()).toContain('Chartreux');
});

// "POR COBRAR" también sale en el ticket de un pedido REAL sin cobrar. Dejarlo aquí haría que los
// dos papeles se parezcan justo donde tienen que distinguirse.
test('no lleva el estado del cobro, que es lo que lo confundiría con un ticket real', () => {
  const html = papel();
  expect(html).not.toContain('POR COBRAR');
  expect(html).not.toContain('PAGADO');
});

// El mensaje del negocio se queda: es identidad, no estado. Sin él el papel parece un borrador y no
// algo que el negocio entregó.
test('conserva el mensaje del negocio', () => {
  expect(papel()).toContain('¡Gracias!');
});

// EL TOTAL DEL PAPEL Y EL QUE SE COBRA SON EL MISMO, ENVÍO INCLUIDO.
//
// Si difieren, el cliente revisa una cifra y paga otra — y lo descubre con el dinero ya en la mano.
test('el total incluye el envío y coincide con lo que se va a cobrar', () => {
  const cuenta = preCuentaDeLaCuenta({
    folioName: 'Chartreux', serviceType: 'domicilio', customerName: 'Ana',
    lineas: [linea()], envio: 20, total: 210,
  }, ahora);
  expect(cuenta.total).toBe('210.00');
  expect(cuenta.deliveryFee).toBe('20.00');
  // El subtotal es lo demás: si el envío se sumara dos veces, esto se caería.
  expect(cuenta.subtotal).toBe('190.00');
});

// El ticket de una venta REAL sigue llevando lo suyo. Sin esto, un cambio en la pre-cuenta podría
// vaciar el ticket que el cliente sí se lleva.
test('el ticket normal conserva su número y su estado de cobro', () => {
  const cuenta = preCuentaDeLaCuenta({
    folioName: 'Chartreux', serviceType: 'mostrador', customerName: '',
    lineas: [linea()], envio: 0, total: 190,
  }, ahora);
  const html = buildReceiptHtml({ ...cuenta, number: 158, paid: true }, negocio, {});
  expect(html).toContain('Pedido #158');
  expect(html).toContain('PAGADO');
  expect(html).not.toContain('** PRE-CUENTA **');
});
