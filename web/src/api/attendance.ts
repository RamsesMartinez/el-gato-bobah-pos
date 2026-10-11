import { api } from './client';

export interface RosterPerson {
  userId: number;
  name: string;
  openSince?: string;
  hasPin: boolean;
}

export interface Roster {
  enabled: boolean;
  people: RosterPerson[];
}

export type ClockKind = 'in' | 'out';

export interface ClockResult {
  kind: ClockKind;
  at: string;
  workedMinutes?: number;
}

export const attendanceApi = {
  roster: () => api.get<Roster>('/attendance/roster'),
  clock: (userId: number, pin: string, expected: ClockKind) =>
    api.post<ClockResult>('/attendance/clock', { userId, pin, expected }),
};

export interface AttendanceSettings {
  enabled: boolean;
  measureSchedule: boolean;
  graceInMin: number;
  graceOutMin: number;
}

export interface ScheduleShift {
  weekday: number;
  shiftNo: number;
  start: number; // minutos desde la medianoche
  end: number;
}

export interface ScheduleVersion {
  effectiveFrom: string;
  days: ScheduleShift[];
}

export interface ReportEntry {
  id: number;
  in: string;
  out?: string;
  status: 'open' | 'closed' | 'missing_out';
  source: 'clock' | 'manual';
  corrected: boolean;
  inIp: string;
  outIp: string;
  inDevice?: Record<string, string>;
  outDevice?: Record<string, string>;
  inDeviceKey: string;
  outDeviceKey: string;
}

export interface ReportDay {
  date: string;
  entries: ReportEntry[];
  workedMinutes: number;
  lateMinutes?: number;
  overtimeMinutes?: number;
  missingOut: boolean;
  noSchedule: boolean;
}

export interface ReportPerson {
  userId: number;
  name: string;
  days: ReportDay[];
  workedMinutes: number;
  lateMinutes?: number;
  overtimeMinutes?: number;
  missingOutDays: number;
}

export interface AttendanceReport {
  measureSchedule: boolean;
  people: ReportPerson[];
}

export const attendanceAdminApi = {
  settings: () => api.get<AttendanceSettings>('/admin/attendance/settings'),
  setSettings: (s: AttendanceSettings) => api.put<AttendanceSettings>('/admin/attendance/settings', s),
  schedules: (userId: number) =>
    api.get<{ tracksAttendance: boolean; versions: ScheduleVersion[] }>(`/admin/attendance/schedules/${userId}`),
  setSchedule: (userId: number, effectiveFrom: string, days: ScheduleShift[]) =>
    api.post(`/admin/attendance/schedules/${userId}`, { effectiveFrom, days }),
  setTracks: (userId: number, tracks: boolean) => api.put(`/admin/attendance/people/${userId}/tracks`, { tracks }),
  report: (from: string, to: string, userId?: number) => {
    const p = new URLSearchParams({ from, to });
    if (userId) p.set('userId', String(userId));
    return api.get<AttendanceReport>(`/admin/attendance/report?${p.toString()}`);
  },
};

export const attendanceCorrectionApi = {
  correct: (entryId: number, field: 'clock_in_at' | 'clock_out_at', at: string, reason: string) =>
    api.post<void>(`/admin/attendance/entries/${entryId}/corrections`, { field, at, reason }),
  manual: (userId: number, inAt: string, outAt: string, reason: string) =>
    api.post<{ id: number }>('/admin/attendance/entries', { userId, in: inAt, out: outAt, reason }),
};
