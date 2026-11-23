import { render, screen } from '@testing-library/react';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';

import { GroupsCell } from './ProductsAdminPage';

// EL GRUPO APAGADO QUE NADIE VEÍA (#289, defecto 3). Un grupo inactivo sigue ligado al producto pero
// no sale en la pantalla de venta, y en la lista de productos el renglón se veía igual que los demás:
// el equipo creyó que la opción no existía. El número sigue contando solo los activos, como antes; la
// insignia dice cuántos más hay apagados, corta para no ensanchar la columna, y el detalle va en title.
const pinta = (groupCount: number, inactiveGroupCount: number) =>
  render(<ChakraProvider value={defaultSystem}><GroupsCell groupCount={groupCount} inactiveGroupCount={inactiveGroupCount} /></ChakraProvider>);

test('con grupos apagados, el número sigue siendo el de los activos y la insignia dice cuántos apagados', () => {
  pinta(4, 2);
  expect(screen.getByText('4')).toBeInTheDocument();
  const badge = screen.getByText('2 inact.');
  expect(badge.closest('[title]')?.getAttribute('title')).toBe('2 grupos inactivos: no salen en la venta');
  expect(getComputedStyle(badge.closest('[title]') as HTMLElement).whiteSpace).toBe('nowrap');
});

test('en singular dice «grupo inactivo» y «sale»', () => {
  pinta(0, 1);
  expect(screen.getByText('1 inact.').closest('[title]')?.getAttribute('title')).toBe('1 grupo inactivo: no sale en la venta');
});

test('sin grupos apagados no hay insignia', () => {
  pinta(3, 0);
  expect(screen.queryByText(/inact\./)).toBeNull();
});
