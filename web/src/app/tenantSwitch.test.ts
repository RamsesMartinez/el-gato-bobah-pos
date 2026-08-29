import { hayQueLimpiar, empresaRecordada, recordarEmpresa } from './tenantSwitch';

beforeEach(() => localStorage.clear());

test('entrar a otra empresa obliga a limpiar', () => {
  expect(hayQueLimpiar(1, 2)).toBe(true);
});

test('volver a la misma empresa no limpia', () => {
  expect(hayQueLimpiar(1, 1)).toBe(false);
});

// El caso que motivó todo esto: un 401 por hipo de red cierra la sesión, el cajero vuelve a entrar
// a la MISMA empresa y su ticket a medias tiene que seguir ahí. Por eso se decide al entrar y
// comparando la empresa, no al salir.
test('un cierre de sesión no pierde el carrito si se vuelve a la misma empresa', () => {
  recordarEmpresa(1);
  expect(hayQueLimpiar(empresaRecordada(), 1)).toBe(false);
});

// Un dispositivo sin marca puede traer un carrito persistido de antes de que existiera la marca.
// Se limpia: el costo es recargar el menú; el de no limpiar es cobrar con el catálogo ajeno.
test('sin marca previa se limpia', () => {
  expect(hayQueLimpiar(null, 2)).toBe(true);
});

test('la marca sobrevive para la siguiente entrada', () => {
  expect(empresaRecordada()).toBeNull();
  recordarEmpresa(7);
  expect(empresaRecordada()).toBe(7);
  expect(hayQueLimpiar(empresaRecordada(), 7)).toBe(false);
  expect(hayQueLimpiar(empresaRecordada(), 8)).toBe(true);
});

test('una marca corrupta se trata como si no hubiera', () => {
  localStorage.setItem('sesion.ultimaEmpresa', 'no-es-un-numero');
  expect(empresaRecordada()).toBeNull();
  expect(hayQueLimpiar(empresaRecordada(), 3)).toBe(true);
});
