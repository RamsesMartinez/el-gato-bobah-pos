import { expect, test } from 'vitest';
import { uuidv5 } from './uuidv5';

// El vector de la RFC 4122 (DNS, "www.example.com"). Si la función se equivoca, el reintento de
// subir las cuentas viejas mandaría otros opId y el servidor duplicaría renglones.
test('coincide con el vector conocido', () => {
  expect(uuidv5('www.example.com', '6ba7b810-9dad-11d1-80b4-00c04fd430c8'))
    .toBe('2ed6657d-e927-568b-95e1-2665a8aea6a2');
});

test('es estable y distingue nombres', () => {
  const ns = '0f8fad5b-d9cb-469f-a165-70867728950e';
  expect(uuidv5('a', ns)).toBe(uuidv5('a', ns));
  expect(uuidv5('a', ns)).not.toBe(uuidv5('b', ns));
});

test('acepta un nombre con acentos (UTF-8)', () => {
  expect(uuidv5('Levkoy·ñ', '6ba7b810-9dad-11d1-80b4-00c04fd430c8')).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
});
