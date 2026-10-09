import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import type { MenuCategory } from '../../types/pos';
import { CategoryRail } from './CategoryRail';

vi.mock('../../hooks/useCatOrder', () => ({
  useCatOrder: () => ({ order: undefined, setRootOrder: vi.fn(), setSubOrder: vi.fn() }),
}));

const cat = (id: number, name: string): MenuCategory =>
  ({ id, name, parentId: null, sortKey: id, color: null, imageUrl: null });
const CATS = ['Bebidas Calientes', 'Bebidas Frías', 'Boneless & Alitas', 'Combos', 'Crepas', 'Desayunos', 'Ramen & Asiática', 'Snacks']
  .map((n, i) => cat(i + 1, n));

// JSDOM NO HACE LAYOUT: el riel mide lo que aquí se diga. 1400 px de categorías en 600 de ancho es
// lo que pasa a 1024×600 con el ticket abierto.
let desplazado = 0;
const scrollBy = vi.fn((o: { left: number }) => { desplazado += o.left; });
beforeEach(() => {
  desplazado = 0;
  const esRiel = (el: HTMLElement) => el.dataset.riel === 'categorias';
  vi.spyOn(HTMLElement.prototype, 'scrollWidth', 'get').mockImplementation(function (this: HTMLElement) { return esRiel(this) ? 1400 : 0; });
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(function (this: HTMLElement) { return esRiel(this) ? 600 : 0; });
  vi.spyOn(HTMLElement.prototype, 'scrollLeft', 'get').mockImplementation(function (this: HTMLElement) { return esRiel(this) ? desplazado : 0; });
  (HTMLElement.prototype as unknown as { scrollBy: typeof scrollBy }).scrollBy = scrollBy;
});
afterEach(() => vi.restoreAllMocks());

function pinta() {
  const qc = new QueryClient();
  render(
    <QueryClientProvider client={qc}>
      <Provider><CategoryRail categories={CATS} selection={{ kind: 'top' }} onSelect={vi.fn()} /></Provider>
    </QueryClientProvider>,
  );
}

// Desayunos, Ramen y Snacks quedaban fuera de la vista sin ninguna señal (validación como usuario
// nuevo): un carrusel sin indicador es una categoría que nadie encuentra.
describe('las categorías que no caben se notan', () => {
  test('con categorías fuera de la vista hay una flecha de 44 px que avanza', async () => {
    pinta();
    const mas = await screen.findByRole('button', { name: 'Más categorías' });
    expect(parseInt(getComputedStyle(mas).minHeight, 10)).toBeGreaterThanOrEqual(44);
    expect(parseInt(getComputedStyle(mas).minWidth, 10)).toBeGreaterThanOrEqual(44);
    fireEvent.click(mas);
    expect(scrollBy).toHaveBeenCalledWith(expect.objectContaining({ left: expect.any(Number) }));
    expect(scrollBy.mock.calls[0][0].left).toBeGreaterThan(0);
  });

  test('al llegar al final la flecha se va y aparece la de regreso', async () => {
    pinta();
    await screen.findByRole('button', { name: 'Más categorías' });
    desplazado = 800;
    act(() => { fireEvent.scroll(screen.getByTestId('riel-categorias')); });
    expect(screen.queryByRole('button', { name: 'Más categorías' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Categorías anteriores' })).toBeInTheDocument();
  });
});
