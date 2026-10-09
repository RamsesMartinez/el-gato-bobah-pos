import { money } from './format';

// Spec 029: «$2,165.2» se lee como un error de captura. Con centavos van los dos; entero, ninguno
// («$45» es como se lee el precio en el mostrador).
test('con centavos, dos decimales; entero, ninguno', () => {
  expect(money('2165.2')).toBe('$2,165.20');
  expect(money(0.5)).toBe('$0.50');
  expect(money('45')).toBe('$45');
  expect(money('-30.67')).toBe('-$30.67');
});
