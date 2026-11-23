import { fireEvent, render, screen } from '@testing-library/react';
import { Provider } from './ui/provider';
import { CatalogImage } from './CatalogImage';

const wrap = (ui: React.ReactElement) => render(<Provider>{ui}</Provider>);
const own = 'https://media.test/2/products/15/aaaa-1200.webp';
const cat = 'https://media.test/2/categories/3/bbbb-1200.webp';

// Un producto sin foto nunca muestra un hueco (FR-017): la cadena es foto propia, foto de su
// categoría, dibujo de su familia y, sin familia, el genérico.
test('con foto propia pide la chica, ofrece la grande y declara su tamaño', () => {
  wrap(<CatalogImage imageUrl={own} categoryImageUrl={cat} family="coffee" size={40} alt="Taro" />);
  const img = screen.getByRole('img', { name: 'Taro' }) as HTMLImageElement;
  expect(img.getAttribute('src')).toBe('https://media.test/2/products/15/aaaa-400.webp');
  expect(img.getAttribute('srcset')).toContain('aaaa-1200.webp 1200w');
  expect(img.getAttribute('sizes')).toBe('40px');
  expect(img.getAttribute('width')).toBe('40');
  expect(img.getAttribute('height')).toBe('40');
  expect(img.getAttribute('loading')).toBe('lazy');
});

test('sin foto propia usa la de su categoría', () => {
  wrap(<CatalogImage imageUrl={null} categoryImageUrl={cat} family="coffee" size={40} alt="Taro" />);
  expect(screen.getByRole('img', { name: 'Taro' }).getAttribute('src')).toContain('/categories/3/bbbb-400.webp');
});

test('sin ninguna foto dibuja la familia, desde el sprite y no una descarga por producto', () => {
  const { container } = wrap(<CatalogImage imageUrl={null} categoryImageUrl={null} family="coffee" size={40} alt="Taro" />);
  expect(container.querySelector('use')?.getAttribute('href')).toBe('/ilustraciones/food-families.v1.svg#coffee');
  expect(screen.getByRole('img', { name: 'Taro' })).toBeTruthy();
});

test('sin familia, el genérico', () => {
  const { container } = wrap(<CatalogImage imageUrl={null} family="" size={40} alt="x" />);
  expect(container.querySelector('use')?.getAttribute('href')).toMatch(/#dish$/);
});

test('una foto que no carga cae al siguiente de la cadena en vez de dejar el ícono roto', () => {
  const { container } = wrap(<CatalogImage imageUrl={own} categoryImageUrl={null} family="tea" size={40} alt="Taro" />);
  fireEvent.error(screen.getByRole('img', { name: 'Taro' }));
  expect(container.querySelector('use')?.getAttribute('href')).toMatch(/#tea$/);
});

test('una foto nueva se muestra aunque la anterior haya fallado', () => {
  // Las URLs llevan la huella del archivo: cada foto nueva es otra URL y merece su intento.
  const { rerender } = wrap(<CatalogImage imageUrl={null} categoryImageUrl={cat} family="tea" size={40} alt="Taro" />);
  fireEvent.error(screen.getByRole('img', { name: 'Taro' }));
  rerender(<Provider><CatalogImage imageUrl={own} categoryImageUrl={cat} family="tea" size={40} alt="Taro" /></Provider>);
  expect(screen.getByRole('img', { name: 'Taro' }).getAttribute('src')).toContain('/products/15/aaaa-400.webp');
});

test('si falló la foto propia y se quita, se ve la de la categoría', () => {
  const { rerender } = wrap(<CatalogImage imageUrl={own} categoryImageUrl={cat} family="tea" size={40} alt="Taro" />);
  fireEvent.error(screen.getByRole('img', { name: 'Taro' }));
  rerender(<Provider><CatalogImage imageUrl={null} categoryImageUrl={cat} family="tea" size={40} alt="Taro" /></Provider>);
  expect(screen.getByRole('img', { name: 'Taro' }).getAttribute('src')).toContain('/categories/3/bbbb-400.webp');
});
