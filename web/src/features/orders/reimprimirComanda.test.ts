import { buildKitchenHtml } from '../../utils/printKitchen';

// LA REIMPRESIÓN SACA EL PEDIDO COMPLETO, NO SOLO LO ÚLTIMO.
//
// Es el camino de recuperación: la comanda que sale sola al agregar lleva SOLO lo nuevo —cocina ya
// está preparando lo anterior—, así que cuando un papel se pierde o la impresora falla, pedir el
// pedido entero es la única forma de que cocina lo tenga todo. Y es el camino que nadie ejercita a
// diario, así que es el que más necesita su test.
test('la reimpresión trae todos los renglones, incluidos los que ya habían salido', () => {
  const pedido = {
    id: 1, number: 12, folioName: 'Tigre', openedAt: new Date().toISOString(), customerName: null,
    lines: [
      { id: 10, productName: 'Alitas', quantity: 1, modifiers: [], notes: '' },
      { id: 11, productName: 'Papas', quantity: 1, modifiers: [], notes: '' },
      { id: 12, productName: 'Refresco', quantity: 2, modifiers: [], notes: '' },
    ],
  } as never;

  // Sin lista de renglones: es la firma que usa la reimpresión del tablero.
  const html = buildKitchenHtml(pedido);

  expect(html).toContain('Alitas');
  expect(html).toContain('Papas');
  expect(html).toContain('Refresco');
  // Y NO va marcada como agregado: es el pedido entero, no un añadido.
  expect(html).not.toContain('AGREGADO');
});
