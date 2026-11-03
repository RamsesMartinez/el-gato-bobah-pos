import { matchesWords, money } from './format';

// Spec 029: «$2,165.2» se lee como un error de captura. Con centavos van los dos; entero, ninguno
// («$45» es como se lee el precio en el mostrador).
test('con centavos, dos decimales; entero, ninguno', () => {
  expect(money('2165.2')).toBe('$2,165.20');
  expect(money(0.5)).toBe('$0.50');
  expect(money('45')).toBe('$45');
  expect(money('-30.67')).toBe('-$30.67');
});

// Spec 045: el buscador del conteo traía «Tapioca» al buscar «apio». Cada palabra buscada tiene que
// ser el inicio de una palabra del nombre, sin acentos ni mayúsculas.
test.each([
  ['Tapioca', 'apio', false],
  ['Apio', 'apio', true],
  ['Leche de almendra', 'almen', true],
  ['Azúcar', 'azucar', true],
  ['Azúcar', 'AZÚ', true],
  ['Leche de almendra', 'leche alm', true],
  ['Leche de almendra', 'leche  alm ', true],
  ['Leche entera', 'leche alm', false],
  ['Bolsa de hielo', 'b', true],
  ['Tapioca', 'p', false],
  ['Tapioca', '', true],
  ['Jarabe (vainilla)', 'vain', true],
  ['Té verde', 'te', true],
])('%s con «%s» → %s', (text, query, want) => {
  expect(matchesWords(text, query)).toBe(want);
});
