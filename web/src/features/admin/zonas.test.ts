import { ZONAS_MEXICO, etiquetaDeZona } from './zonas';

test('la lista trae la zona por default del sistema', () => {
  expect(ZONAS_MEXICO.some((z) => z.value === 'America/Mexico_City')).toBe(true);
});

test('todas las zonas de la lista son nombres IANA que el navegador reconoce', () => {
  // Si una estuviera mal escrita, el servidor la rechazaría al guardar y el operador vería un
  // error sin entender por qué: la opción que le ofrecimos no existe.
  for (const z of ZONAS_MEXICO) {
    expect(() => new Intl.DateTimeFormat('es-MX', { timeZone: z.value })).not.toThrow();
  }
});

test('una zona fuera de la lista se muestra tal cual, no vacía', () => {
  expect(etiquetaDeZona('Europe/Madrid')).toBe('Europe/Madrid');
});

test('una zona de la lista se muestra con su nombre legible', () => {
  expect(etiquetaDeZona('America/Cancun')).toContain('Canc');
});
