import { matchedAlias, matchesWords, money, normalize } from './format';
import { describe } from 'vitest';

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

// Spec 063: el alternativo que se señala es el primero que coincide, en el orden capturado, con la
// misma regla de coincidencia que el buscador usa para el nombre.
describe('matchedAlias', () => {
  const contains = (text: string, q: string) => normalize(text).includes(normalize(q.trim()));
  test('el primero que coincide, sin acentos ni mayúsculas', () => {
    expect(matchedAlias(['Café helado', 'Café frío'], 'CAFE', contains)).toBe('Café helado');
    expect(matchedAlias(['Café helado', 'Café frío'], 'frio', contains)).toBe('Café frío');
  });
  test('ninguno, vacío o búsqueda en blanco no señala nada', () => {
    expect(matchedAlias(['Café frío'], 'té', contains)).toBeUndefined();
    expect(matchedAlias(undefined, 'café', contains)).toBeUndefined();
    expect(matchedAlias(['Café frío'], '   ', contains)).toBeUndefined();
  });
  test('usa la regla del buscador: por inicio de palabra, «apio» no es «Tapioca»', () => {
    expect(matchedAlias(['Perlas de tapioca'], 'apio', matchesWords)).toBeUndefined();
    expect(matchedAlias(['Perlas de tapioca'], 'tapi', matchesWords)).toBe('Perlas de tapioca');
  });
});
