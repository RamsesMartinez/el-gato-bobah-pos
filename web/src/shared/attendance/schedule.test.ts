import { expect, test } from 'vitest';
import { copyToWeekdays, crossesMidnight, emptyWeek, shiftsFromWeek, weekFromShifts } from './schedule';
import { clockLabel } from './schedule';

test('copiar a lunes–viernes no toca el fin de semana', () => {
  const w = emptyWeek();
  w[1] = [{ start: 540, end: 1020 }];
  w[6] = [{ start: 600, end: 900 }];
  const out = copyToWeekdays(w);
  expect(out[5]).toEqual([{ start: 540, end: 1020 }]);
  expect(out[6]).toEqual([{ start: 600, end: 900 }]);
  expect(out[0]).toEqual([]);
});

test('el segundo turno del día se guarda como turno 2', () => {
  const w = emptyWeek();
  w[5] = [{ start: 540, end: 1020 }, { start: 1320, end: 360 }];
  const shifts = shiftsFromWeek(w);
  expect(shifts).toEqual([
    { weekday: 5, shiftNo: 1, start: 540, end: 1020 },
    { weekday: 5, shiftNo: 2, start: 1320, end: 360 },
  ]);
  expect(weekFromShifts(shifts)[5]).toEqual(w[5]);
});

test('22:00 a 6:00 termina al día siguiente', () => {
  expect(crossesMidnight({ start: 1320, end: 360 })).toBe(true);
  expect(crossesMidnight({ start: 540, end: 1020 })).toBe(false);
});

test('las horas del selector se escriben como en el reporte, en 12 horas', () => {
  expect(clockLabel(0)).toBe('12:00 a.m.');
  expect(clockLabel(6 * 60)).toBe('06:00 a.m.');
  expect(clockLabel(12 * 60)).toBe('12:00 p.m.');
  expect(clockLabel(15 * 60 + 45)).toBe('03:45 p.m.');
});
