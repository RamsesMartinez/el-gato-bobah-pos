import type { ReportPerson } from '../../api/attendance';

export type ReportView = 'persona' | 'dia' | 'semana';

const KEY = 'egb:asistencia:vista';

// La tableta recuerda la última vista. Leer o escribir el almacenamiento puede tronar (ventana
// privada): entonces vale «por persona».
export function loadView(): ReportView {
  try {
    const v = localStorage.getItem(KEY);
    if (v === 'persona' || v === 'dia' || v === 'semana') return v;
  } catch { /* sin almacenamiento */ }
  return 'persona';
}

export function saveView(v: ReportView): void {
  try { localStorage.setItem(KEY, v); } catch { /* sin almacenamiento */ }
}

// La cuadrícula de semana solo tiene sentido con un rango de hasta 7 días.
export function weekViewAllowed(from: string, to: string): boolean {
  if (!from || !to) return false;
  const dayLabels = (Date.parse(to) - Date.parse(from)) / 86_400_000;
  return dayLabels >= 0 && dayLabels < 7;
}

export function daysBetween(from: string, to: string): string[] {
  const out: string[] = [];
  for (let t = Date.parse(`${from}T12:00:00Z`); t <= Date.parse(`${to}T12:00:00Z`); t += 86_400_000) {
    out.push(new Date(t).toISOString().slice(0, 10));
  }
  return out;
}

export function minutesLabel(min: number): string {
  return `${Math.floor(min / 60)} h ${String(min % 60).padStart(2, '0')}`;
}

// rowsByDay aplana el reporte para la vista «día por día».
export function rowsByDay(people: ReportPerson[]) {
  return people.flatMap((p) => p.days.map((d) => ({ person: p, day: d })))
    .sort((a, b) => a.day.date.localeCompare(b.day.date) || a.person.name.localeCompare(b.person.name));
}

function zoneOffsetMs(t: number, zone: string): number {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: zone, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  }).formatToParts(new Date(t));
  const get = (k: string) => Number(parts.find((p) => p.type === k)?.value);
  return Date.UTC(get('year'), get('month') - 1, get('day'), get('hour'), get('minute'), get('second')) - t;
}

// instantInZone convierte «este día a esta hora, en la zona del negocio» al instante que se manda
// al servidor. La hora del navegador no cuenta: dos tabletas con el reloj distinto capturarían
// instantes distintos para la misma hora.
export function instantInZone(date: string, minutes: number, zone: string): string {
  const [y, m, d] = date.split('-').map(Number);
  const guess = Date.UTC(y, m - 1, d, Math.floor(minutes / 60), minutes % 60);
  return new Date(guess - zoneOffsetMs(guess, zone)).toISOString();
}
