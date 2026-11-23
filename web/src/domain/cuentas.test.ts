import { describe, expect, test } from 'vitest';
import type { AccountState } from '../types/pos';
import { cuenta } from '../features/pos/__fixtures__/cuentas';
import { missingFor } from './cuentas';

// Lo que le falta a una cuenta para quitarse de la fila (spec 089, research R-1). El estado lo decide
// el servidor; aquí solo se traduce. Una «pagada» que sigue en la fila es la que no se ha entregado,
// y eso es justo lo que la pantalla tiene que decir.
describe('lo que le falta a cada cuenta', () => {
  test.each<[AccountState, Partial<Parameters<typeof cuenta>[0]>, string, string, boolean, boolean]>([
    ['capturing', { kind: 'draft', orderId: null, total: '95.00', outstanding: '95.00' }, 'Sin enviar · $95', 'Sin enviar · $95', false, false],
    ['in_kitchen', { total: '180.00', outstanding: '180.00' }, 'Falta cobrar $180 y entregar', 'Cobrar $180 · entregar', true, true],
    ['paid_in_kitchen', { total: '120.00', paid: '120.00', outstanding: '0.00' }, 'Falta entregar', 'Falta entregar', false, true],
    ['partly_paid', { total: '240.00', paid: '100.00', outstanding: '140.00' }, 'Falta cobrar $140 y entregar', 'Cobrar $140 · entregar', true, true],
    ['delivered_owes', { total: '60.50', outstanding: '60.50' }, 'Falta cobrar $60.50', 'Cobrar $60.50', true, false],
    ['closed_with_new', { outstanding: '0.00' }, 'Nuevo sin enviar', 'Nuevo sin enviar', false, false],
  ])('%s', (state, over, texto, corto, cobrar, entregar) => {
    const f = missingFor(cuenta({ key: 'k', state, ...over }));
    expect(f.text).toBe(texto);
    expect(f.short).toBe(corto);
    expect(f.charge).toBe(cobrar);
    expect(f.deliver).toBe(entregar);
  });

  test('el monto que falta es el del servidor (outstanding), no el total', () => {
    const f = missingFor(cuenta({ key: 'k', state: 'partly_paid', total: '300.00', paid: '150.00', outstanding: '150.00' }));
    expect(f.text).toContain('$150');
    expect(f.text).not.toContain('$300');
  });

  test('ninguna cuenta dice «cobrar $0»: una pagada solo falta entregarla', () => {
    for (const state of ['paid_in_kitchen', 'closed_with_new'] as const) {
      expect(missingFor(cuenta({ key: 'k', state, outstanding: '0.00' })).text).not.toMatch(/\$0/);
    }
  });

  test('una cuenta en captura no ofrece acciones aunque el estado dijera otra cosa', () => {
    const f = missingFor(cuenta({ key: 'k', kind: 'draft', orderId: null, state: 'capturing' }));
    expect(f.charge || f.deliver).toBe(false);
  });

  test('el color dice qué tan urgente es: rojo si falta dinero, azul si solo falta entregar', () => {
    expect(missingFor(cuenta({ key: 'k', state: 'in_kitchen' })).color).toBe('red');
    expect(missingFor(cuenta({ key: 'k', state: 'delivered_owes' })).color).toBe('red');
    expect(missingFor(cuenta({ key: 'k', state: 'paid_in_kitchen', outstanding: '0.00' })).color).toBe('blue');
    expect(missingFor(cuenta({ key: 'k', state: 'capturing', kind: 'draft' })).color).toBe('gray');
  });
});
