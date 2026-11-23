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

// Spec 063 (maqueta «Coincidió con», opción B, revisión de tableta M3): el otro nombre que coincidió
// va en una etiqueta en el lugar de la descripción, para que el mosaico no crezca mientras se busca.
describe('el otro nombre que coincidió', () => {
  const ICE = { id: 2, name: 'Ice Coffee', description: 'Café americano con hielo', price: '65', categoryId: 3, groups: [], altNames: ['Café frío'] } as unknown as MenuProduct;
  test('se ve la etiqueta y no la descripción', () => {
    render(<Provider><ProductTile product={ICE} count={0} onTap={vi.fn()} showPrice price={65} esManual={false} matchedAlt="Café frío" /></Provider>);
    expect(screen.getByText('Ice Coffee')).toBeInTheDocument();
    expect(screen.getByText(/también: café frío/i)).toBeInTheDocument();
    expect(screen.queryByText('Café americano con hielo')).not.toBeInTheDocument();
  });
  test('sin búsqueda por otro nombre, la descripción de siempre y ninguna etiqueta', () => {
    render(<Provider><ProductTile product={ICE} count={0} onTap={vi.fn()} showPrice price={65} esManual={false} /></Provider>);
    expect(screen.getByText('Café americano con hielo')).toBeInTheDocument();
    expect(screen.queryByText(/también:/i)).not.toBeInTheDocument();
  });
});

// Un producto agotado (spec 092) se ve con su etiqueta y no se agrega a la cuenta; tocarlo avisa en
// vez de no hacer nada (un toque mudo se repite).
describe('el mosaico de un producto agotado', () => {
  test('dice «Agotado», no llama a agregar y avisa', async () => {
    const onTap = vi.fn();
    const onSoldOutTap = vi.fn();
    render(
      <Provider>
        <ProductTile product={BUBBLE} count={0} onTap={onTap} showPrice price={20} esManual={false} soldOut onSoldOutTap={onSoldOutTap} />
      </Provider>,
    );
    expect(screen.getByText('Agotado')).toBeInTheDocument();
    const tile = screen.getByRole('button', { name: /bubble gum durazno/i });
    tile.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
    tile.dispatchEvent(new PointerEvent('pointerup', { bubbles: true }));
    tile.click();
    expect(onTap).not.toHaveBeenCalled();
    expect(onSoldOutTap).toHaveBeenCalledWith(BUBBLE);
  });

  test('sin agotar no hay etiqueta', () => {
    pinta(0);
    expect(screen.queryByText('Agotado')).not.toBeInTheDocument();
  });
});
