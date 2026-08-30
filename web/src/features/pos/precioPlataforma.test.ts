import { precioDeLista, deltaDeLista, desglosePrecio, nombreDeLista, MOSTRADOR } from './precioPlataforma';
import type { Menu } from '../../types/pos';

const menu = {
  version: 1,
  categories: [],
  products: [],
  platforms: [
    { id: 5, name: 'Uber Eats', markupPct: '35' },
    { id: 8, name: 'Propio', markupPct: '0' },
  ],
  platformPrices: { 5: { 77: '149' } },
  platformModPrices: { 5: { 300: '30' } },
} as unknown as Menu;

test('en mostrador el precio es el base, sin tocar', () => {
  expect(precioDeLista(menu, MOSTRADOR, 77, 434.98)).toBe(434.98);
});

// Los dos productos reales que destaparon el redondeo en el backend. Si el cliente y el servidor
// no coinciden aquí, el ticket sale con un centavo distinto al cobrado.
test('el margen se aplica y se redondea a 2 decimales, igual que el servidor', () => {
  expect(precioDeLista(menu, 5, 999, 434.98)).toBe(587.22);
  expect(precioDeLista(menu, 5, 998, 398.98)).toBe(538.62);
});

test('el precio capturado a mano gana sobre el calculado', () => {
  expect(precioDeLista(menu, 5, 77, 100)).toBe(149);
});

test('una plataforma sin margen devuelve el base', () => {
  expect(precioDeLista(menu, 8, 77, 100)).toBe(100);
});

test('un extra sin costo sigue sin costo aunque la plataforma tenga margen', () => {
  expect(deltaDeLista(menu, 5, 999, 0)).toBe(0);
});

test('el delta capturado a mano gana', () => {
  expect(deltaDeLista(menu, 5, 300, 20)).toBe(30);
});

test('el indicador nunca queda vacío', () => {
  expect(nombreDeLista(menu, MOSTRADOR)).toBe('Mostrador');
  expect(nombreDeLista(menu, 5)).toBe('Uber Eats');
  // Una lista que ya no existe (el menú se recargó) cae a mostrador en vez de quedar en blanco.
  expect(nombreDeLista(menu, 999)).toBe('Mostrador');
});

test('sin menú cargado todavía, el precio es el base', () => {
  expect(precioDeLista(undefined, 5, 77, 100)).toBe(100);
});

// desglosePrecio alimenta el diálogo de captura: el operador tiene que ver de dónde sale el número
// que va a corregir, o corrige a ciegas.
test('desglosePrecio distingue el calculado del capturado a mano', () => {
  const calculado = desglosePrecio(menu, 5, 999, 434.98);
  expect(calculado).toEqual({ base: 434.98, calculado: 587.22, vigente: 587.22, esManual: false });

  const manual = desglosePrecio(menu, 5, 77, 100);
  expect(manual).toEqual({ base: 100, calculado: 135, vigente: 149, esManual: true });
});

// Un precio manual IGUAL al calculado sigue siendo manual: si no se distinguiera, el botón de
// quitarlo desaparecería y la excepción quedaría atrapada en la base.
test('un precio manual que coincide con el calculado se sigue viendo como manual', () => {
  const conIgual = {
    ...menu,
    platformPrices: { 5: { 999: '587.22' } },
  } as unknown as Menu;
  expect(desglosePrecio(conIgual, 5, 999, 434.98)?.esManual).toBe(true);
});

test('en mostrador no hay nada que capturar', () => {
  expect(desglosePrecio(menu, MOSTRADOR, 77, 100)).toBeNull();
});
