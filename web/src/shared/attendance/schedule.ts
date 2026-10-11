import type { ScheduleShift } from '../../api/attendance';

// Los días en el orden en que se leen (lunes primero). El número es el de Go/JS: domingo = 0.
export const WEEK: Array<{ weekday: number; label: string }> = [
  { weekday: 1, label: 'Lunes' }, { weekday: 2, label: 'Martes' }, { weekday: 3, label: 'Miércoles' },
  { weekday: 4, label: 'Jueves' }, { weekday: 5, label: 'Viernes' }, { weekday: 6, label: 'Sábado' },
  { weekday: 0, label: 'Domingo' },
];

export type Week = Record<number, Array<{ start: number; end: number }>>;

export function emptyWeek(): Week {
  return { 0: [], 1: [], 2: [], 3: [], 4: [], 5: [], 6: [] };
}

export function weekFromShifts(shifts: ScheduleShift[]): Week {
  const w = emptyWeek();
  for (const s of [...shifts].sort((a, b) => a.shiftNo - b.shiftNo)) w[s.weekday].push({ start: s.start, end: s.end });
  return w;
}

export function shiftsFromWeek(w: Week): ScheduleShift[] {
  return WEEK.flatMap(({ weekday }) => w[weekday].map((s, i) => ({ weekday, shiftNo: i + 1, start: s.start, end: s.end })));
}

// copyToWeekdays copia el horario del lunes de martes a viernes, sin tocar el fin de semana.
export function copyToWeekdays(w: Week): Week {
  const out = { ...w };
  for (const d of [2, 3, 4, 5]) out[d] = w[1].map((s) => ({ ...s }));
  return out;
}

// Un turno cuya salida no es posterior a la entrada termina al día siguiente.
export function crossesMidnight(s: { start: number; end: number }): boolean {
  return s.end <= s.start;
}

export function hhmm(min: number): string {
  return `${Math.floor(min / 60)}:${String(min % 60).padStart(2, '0')}`;
}

// Las horas del selector: de 15 en 15 minutos en todo el día.
export const QUARTERS = Array.from({ length: 96 }, (_, i) => ({ value: String(i * 15), label: hhmm(i * 15) }));
