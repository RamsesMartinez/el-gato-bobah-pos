import { describe, expect, it } from 'vitest';
import { buildKitchenHtml, tamanoDelFolio } from './printKitchen';
import type { ReceiptOrder } from '../types/pos';

const pedido: ReceiptOrder = {
  folioName: 'Tigre',
  id: 1, number: 14, status: 'abierta', serviceType: 'mostrador', customerName: 'Sánchez',
  subtotal: '275', deliveryFee: '0', total: '275', currency: 'MXN', paid: true,
  openedAt: '2026-08-31T18:27:10Z',
  lines: [
    {
      productName: 'Boneless J', quantity: '2', unitPrice: '200', lineTotal: '430', notes: 'sin apio',
      modifiers: [
        { name: 'Mango habanero', quantity: 2, priceDelta: '0' },
        { name: 'Aderezo ranch', quantity: 1, priceDelta: '15' },
      ],
    },
    { productName: 'Refresco', quantity: '1', unitPrice: '25', lineTotal: '25', modifiers: [] },
  ],
};

describe('comanda de cocina', () => {
  const html = buildKitchenHtml(pedido);

  // Es el reemplazo de la libreta: quien cocina busca el número desde lejos.
  it('el folio va grande y arriba', () => {
    expect(html).toContain('#14');
  });

  it('lista lo que hay que preparar, con cantidades', () => {
    expect(html).toContain('2x Boneless J');
    expect(html).toContain('Mango habanero');
    expect(html).toContain('x2');
    expect(html).toContain('Refresco');
  });

  // La nota de la línea es la que cambia cómo se prepara. Perderla es servir mal el plato.
  it('conserva las notas de la línea', () => {
    expect(html).toContain('sin apio');
  });

  // SIN precios ni total: es el papel de cocina, no el del cliente. Si llevara importes y alguien
  // se lo entrega al cliente, se vuelve un comprobante que el negocio no emitió.
  it('no lleva precios ni total', () => {
    expect(html).not.toContain('275');
    expect(html).not.toContain('200');
    expect(html).not.toContain('$');
    expect(html).not.toMatch(/total/i);
  });

  // Los adicionales SIN costo también salen: son justo los que cambian la preparación ("sin
  // cebolla"). El ajuste que los oculta es del ticket del cliente, para acortar el papel.
  it('incluye los adicionales aunque no cuesten', () => {
    expect(html).toContain('Mango habanero');
  });

  it('marca a quién es cuando hay nombre', () => {
    expect(html).toContain('Sánchez');
  });
});

// LA COMANDA DEL AGREGADO LLEVA SOLO LO AGREGADO.
//
// Cocina ya está preparando lo anterior. Un papel con el pedido entero la haría prepararlo dos
// veces, y ese desperdicio no se ve hasta que sale la comida de más. El folio es el mismo en los
// dos papeles: es con lo que cocina los junta.
test('la comanda de un agregado trae solo los renglones nuevos, con el mismo folio', () => {
  const pedido = {
    id: 1, number: 12, folioName: 'Tigre', openedAt: new Date().toISOString(),
    customerName: null,
    lines: [
      { id: 10, productName: 'Alitas', quantity: 1, modifiers: [], notes: '' },
      { id: 11, productName: 'Papas', quantity: 1, modifiers: [], notes: '' },
      { id: 12, productName: 'Refresco', quantity: 2, modifiers: [], notes: '' },
    ],
  } as never;

  const html = buildKitchenHtml(pedido, [12]);

  expect(html).toContain('Refresco');
  expect(html).not.toContain('Alitas');
  expect(html).not.toContain('Papas');
  expect(html).toContain('Tigre');
  expect(html).toContain('AGREGADO');
  // Sin precios, igual que la comanda completa: un papel con importes que llegue al cliente sería
  // un comprobante que el negocio no emitió.
  expect(html).not.toMatch(/\$\s?\d/);
});

// Sin la lista, sigue saliendo el pedido completo: es la comanda del confirmado y la reimpresión.
test('sin lista de renglones sale el pedido completo y sin la marca de agregado', () => {
  const pedido = {
    id: 1, number: 12, folioName: 'Tigre', openedAt: new Date().toISOString(),
    customerName: null,
    lines: [
      { id: 10, productName: 'Alitas', quantity: 1, modifiers: [], notes: '' },
      { id: 12, productName: 'Refresco', quantity: 2, modifiers: [], notes: '' },
    ],
  } as never;

  const html = buildKitchenHtml(pedido);
  expect(html).toContain('Alitas');
  expect(html).toContain('Refresco');
  expect(html).not.toContain('AGREGADO');
});

// EL NOMBRE DE LA RAZA NO SE PUEDE SALIR DEL PAPEL.
//
// La comanda imprime el folio a 46 px sobre un rollo de 58 mm, que da para unas ocho letras. Con
// razas de gato el nombre llega a veinte ("Colorpoint Shorthair") y a ese tamaño la impresora lo
// corta: el operador se queda con "Colorpoi" y el papel deja de servir para cantar el pedido.
//
// Se prueba la MEDIDA y no el HTML porque es donde vive la decisión; que el HTML la use lo cubre el
// caso de abajo.
describe('el folio de la comanda cabe en el rollo', () => {
  // 189 px útiles / (0.6 em × letras de la palabra más larga).
  const cabe = (nombre: string) => {
    const masLarga = nombre.split(/[\s-]+/).reduce((n, p) => Math.max(n, p.length), 0);
    return 0.6 * masLarga * tamanoDelFolio(nombre) <= 189;
  };

  it('los nombres cortos siguen saliendo del tamaño de siempre', () => {
    expect(tamanoDelFolio('Persa')).toBe(46);
    expect(tamanoDelFolio('Tigre')).toBe(46);
    // Ocho letras es lo que cabía antes a 46 px, y sigue cabiendo.
    expect(tamanoDelFolio('Abisinio')).toBe(39);
  });

  it('ninguna raza de la lista se sale del papel', () => {
    const razas = [
      'Colorpoint Shorthair', 'Sagrado de Birmania', 'Neva Masquerade', 'British Shorthair',
      'Levkoy Ucraniano', 'Kurilian Bobtail', 'Bosque de Noruega', 'York Chocolate', 'Pixie-bob',
      'Maine Coon', 'Ojos Azules', 'Persa', 'Manx', 'Mau Egipcio',
    ];
    for (const r of razas) {
      expect(cabe(r), `"${r}" se sale del rollo de 58 mm`).toBe(true);
    }
  });

  // Un tamaño de cero o negativo dejaría el folio invisible, que es peor que cortado: el papel sale
  // sin con qué identificar el pedido.
  it('nunca baja de un tamaño legible', () => {
    expect(tamanoDelFolio('Supercalifragilisticoespialidoso')).toBeGreaterThanOrEqual(18);
    expect(tamanoDelFolio('')).toBe(46);
  });
});
