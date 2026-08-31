import { describe, expect, it } from 'vitest';
import { buildKitchenHtml } from './printKitchen';
import type { OrderView } from '../types/pos';

const pedido: OrderView = {
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
