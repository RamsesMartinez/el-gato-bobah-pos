import { vi } from 'vitest';
import { deviceHeaders, deviceKey, resetDeviceOriginForTests } from './origin';

const KEY = 'egb:device:v1';

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
  resetDeviceOriginForTests();
});

// La clave identifica al navegador en la bitácora: se genera una vez y se reusa, o cada request
// parecería venir de un aparato distinto.
test('la clave se genera una vez y se reusa', () => {
  const k = deviceKey();
  expect(k).toMatch(/^[0-9a-f]{32}$/);
  resetDeviceOriginForTests();
  expect(deviceKey()).toBe(k);
  expect(localStorage.getItem(KEY)).toBe(k);
});

test('una clave guardada que no tiene la forma se reemplaza', () => {
  localStorage.setItem(KEY, 'basura');
  expect(deviceKey()).toMatch(/^[0-9a-f]{32}$/);
});

// En una ventana privada o con el almacenamiento bloqueado, leer o escribir truena. Eso NO puede
// tumbar una venta: el request sale igual, con una clave que dura lo que dure la pestaña.
test('si el almacenamiento truena, los headers salen igual', () => {
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('bloqueado'); });
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('bloqueado'); });
  const h = deviceHeaders();
  expect(h['X-Device-Key']).toMatch(/^[0-9a-f]{32}$/);
  expect(() => JSON.parse(h['X-Device-Info'])).not.toThrow();
});

test('la descripción lleva navegador y pantalla en un JSON corto', () => {
  const info = JSON.parse(deviceHeaders()['X-Device-Info']) as Record<string, string>;
  expect(info.screen).toMatch(/^\d+x\d+$/);
  expect(typeof info.browser).toBe('string');
  expect(deviceHeaders()['X-Device-Info'].length).toBeLessThanOrEqual(300);
});
