import { describe, expect, test, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Provider } from '../../components/ui/provider';
import type { MenuProduct } from '../../types/pos';
import { ProductTile } from './ProductTile';

const BUBBLE = { id: 1, name: 'Bubble Gum Durazno', price: '20.00', categoryId: 3, groups: [] } as unknown as MenuProduct;

function pinta(count: number) {
  render(
    <Provider>
      <ProductTile product={BUBBLE} count={count} onTap={vi.fn()} showPrice price={20} esManual={false} />
    </Provider>,
  );
}

// El «1×» iba encima del nombre: «Bubble Gum Durazn» con la última letra tapada (validación como
// usuario nuevo). Con algo en la cuenta, el nombre deja el hueco del contador.
describe('el contador del mosaico no tapa el nombre', () => {
  test('con el contador, el nombre deja su hueco a la derecha', () => {
    pinta(1);
    expect(screen.getByText('1×')).toBeInTheDocument();
    expect(parseInt(getComputedStyle(screen.getByText('Bubble Gum Durazno')).paddingRight, 10)).toBeGreaterThanOrEqual(28);
  });

  test('sin contador, el nombre usa todo el ancho', () => {
    pinta(0);
    expect(parseInt(getComputedStyle(screen.getByText('Bubble Gum Durazno')).paddingRight || '0', 10) || 0).toBe(0);
  });
});
