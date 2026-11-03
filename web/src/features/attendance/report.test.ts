import { vi, expect, test, afterEach } from 'vitest';
import { daysBetween, loadView, saveView, weekViewAllowed } from './report';
import { quarterOfDay } from './report';

afterEach(() => { vi.restoreAllMocks(); localStorage.clear(); });

test('la vista se recuerda y por omisión es «por persona»', () => {
  expect(loadView()).toBe('persona');
  saveView('semana');
  expect(loadView()).toBe('semana');
});

test('si el almacenamiento truena, la pantalla sigue con «por persona»', () => {
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('bloqueado'); });
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('bloqueado'); });
  expect(() => saveView('dia')).not.toThrow();
  expect(loadView()).toBe('persona');
});

test('la semana solo se ofrece con un rango de hasta siete días', () => {
  expect(weekViewAllowed('2026-10-12', '2026-10-18')).toBe(true);
  expect(weekViewAllowed('2026-10-12', '2026-10-19')).toBe(false);
  expect(weekViewAllowed('2026-10-12', '')).toBe(false);
  expect(daysBetween('2026-10-12', '2026-10-14')).toEqual(['2026-10-12', '2026-10-13', '2026-10-14']);
});

test('la hora capturada es la de la zona del negocio, no la del navegador', async () => {
  const { instantInZone } = await import('./report');
  expect(instantInZone('2026-10-12', 9 * 60, 'America/Mexico_City')).toBe('2026-10-12T15:00:00.000Z');
  expect(instantInZone('2026-10-12', 23 * 60 + 30, 'America/Mexico_City')).toBe('2026-10-13T05:30:00.000Z');
});

test('quarterOfDay da la hora del registro en la zona del negocio, bajada al cuarto de hora', () => {
  expect(quarterOfDay('2026-10-12T23:07:00Z', 'America/Mexico_City')).toBe(17 * 60);
  expect(quarterOfDay('2026-10-12T15:44:00Z', 'America/Mexico_City')).toBe(9 * 60 + 30);
});
