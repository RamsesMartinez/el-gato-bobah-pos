import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { vi, expect, test, beforeEach } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { ReportTab } from './ReportTab';

const session = vi.hoisted(() => ({ current: { id: 9, role: 'gerente', permissions: ['attendance.report', 'attendance.correct_others', 'audit.view'] } }));
vi.mock('../../stores/session', () => ({
  useSessionStore: Object.assign(
    (sel: (s: unknown) => unknown) => sel({ user: session.current }),
    { getState: () => ({ user: session.current }) },
  ),
}));
vi.mock('../../hooks/useHoraDelNegocio', () => ({
  useHoraDelNegocio: () => ({ zona: 'America/Mexico_City', diaDelNegocio: () => '2026-10-14' }),
}));
vi.mock('../../api/audit', () => ({ auditApi: { actors: () => Promise.resolve([{ id: 1, name: 'Ana' }, { id: 9, name: 'Gerente' }]) } }));
const report = vi.hoisted(() => vi.fn());
const correct = vi.hoisted(() => vi.fn((..._a: unknown[]) => Promise.resolve()));
vi.mock('../../api/attendance', () => ({
  attendanceAdminApi: { report: (...a: unknown[]) => report(...a) },
  attendanceCorrectionApi: { correct: (...a: unknown[]) => correct(...(a as [])), manual: vi.fn() },
}));

const dayOf = (date: string, userIn: string, out?: string) => ({
  date, workedMinutes: out ? 480 : 0, missingOut: !out, noSchedule: false,
  entries: [{ id: 1, in: userIn, out, status: out ? 'closed' : 'missing_out', source: 'clock', corrected: false,
    inIp: '187.190.4.12', outIp: '', inDeviceKey: 'a3f9b0c1', outDeviceKey: '' }],
});

beforeEach(() => {
  report.mockReset().mockResolvedValue({
    measureSchedule: false,
    people: [
      { userId: 1, name: 'Ana', workedMinutes: 480, missingOutDays: 1, days: [dayOf('2026-10-12', '2026-10-12T15:00:00Z', '2026-10-12T23:00:00Z'), dayOf('2026-10-13', '2026-10-13T15:00:00Z')] },
      { userId: 9, name: 'Gerente', workedMinutes: 480, missingOutDays: 0, days: [dayOf('2026-10-12', '2026-10-12T15:00:00Z', '2026-10-12T23:00:00Z')] },
    ],
  });
});

function renderView() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><ReportTab /></Provider></QueryClientProvider>);
}

test('por omisión agrupa por persona, con lo pendiente en texto', async () => {
  renderView();
  expect(await screen.findByText('1 sin salida')).toBeInTheDocument();
  expect(screen.queryByText('Retardo')).toBeNull();
});

test('día por día muestra cada registro y la semana se arma por días', async () => {
  renderView();
  await screen.findByText('Ana');
  fireEvent.click(screen.getByRole('button', { name: 'Día por día' }));
  expect(await screen.findAllByText('lun 12 oct')).toHaveLength(2);
  fireEvent.click(screen.getByRole('button', { name: 'Semana' }));
  expect(await screen.findByText('mar 13')).toBeInTheDocument();
});

test('el gerente corrige a otros pero no ve «Corregir» en sus propios registros', async () => {
  renderView();
  fireEvent.click(await screen.findByRole('button', { name: 'Día por día' }));
  const rows = await screen.findAllByText('Gerente');
  fireEvent.click(rows[rows.length - 1]);
  expect(await screen.findByText('Gerente · lun 12 oct')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Corregir entrada' })).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Cerrar' }));
  fireEvent.click(screen.getAllByText('Ana')[0]);
  expect(await screen.findByRole('button', { name: 'Corregir entrada' })).toBeInTheDocument();
  expect(screen.getByText(/187\.190\.4\.12/)).toBeInTheDocument();
});

// Turno nocturno: entró a las 22:00 y se corrige la salida a las 6:00. Es la mañana SIGUIENTE, no
// las 6:00 del mismo día, que quedaría antes de la entrada.
test('corregir la salida de un turno nocturno la pone al día siguiente', async () => {
  report.mockResolvedValue({ measureSchedule: false, people: [
    { userId: 1, name: 'Ana', workedMinutes: 0, missingOutDays: 1, days: [dayOf('2026-10-12', '2026-10-13T04:00:00Z')] },
  ] });
  renderView();
  fireEvent.click(await screen.findByRole('button', { name: 'Día por día' }));
  fireEvent.click(await screen.findByText('Ana'));
  fireEvent.click(await screen.findByRole('button', { name: 'Capturar salida' }));
  fireEvent.click(screen.getByRole('button', { name: 'Hora' }));
  fireEvent.click(await screen.findByRole('button', { name: '06:00 a.m.' }));
  fireEvent.change(screen.getByPlaceholderText('Motivo (obligatorio)'), { target: { value: 'Olvidó checar' } });
  fireEvent.click(await screen.findByRole('button', { name: 'Guardar corrección' }));
  await waitFor(() => expect(correct).toHaveBeenCalledWith(1, 'clock_out_at', '2026-10-13T12:00:00.000Z', 'Olvidó checar'));
});

// «Día por día» juntaba dos registros en un renglón (02:45 → 03:00) y escondía el hueco entre ellos.
test('día por día muestra cada registro del día, no la primera entrada y la última salida', async () => {
  report.mockResolvedValue({ measureSchedule: false, people: [{ userId: 1, name: 'Ana', workedMinutes: 12, missingOutDays: 0, days: [{
    date: '2026-10-12', workedMinutes: 12, missingOut: false, noSchedule: false, entries: [
      { id: 1, in: '2026-10-12T20:45:00Z', out: '2026-10-12T20:45:30Z', status: 'closed', source: 'clock', corrected: false },
      { id: 2, in: '2026-10-12T20:47:00Z', out: '2026-10-12T21:00:00Z', status: 'closed', source: 'clock', corrected: false },
    ] }] }] });
  renderView();
  fireEvent.click(await screen.findByRole('button', { name: 'Día por día' }));
  expect(await screen.findByText('02:45 p.m. – 02:45 p.m.')).toBeInTheDocument();
  expect(screen.getByText('02:47 p.m. – 03:00 p.m.')).toBeInTheDocument();
});

test('ninguna columna queda sin título ni se llama «Pendiente»', async () => {
  renderView();
  await screen.findAllByRole('cell', { name: 'Ana' });
  expect(screen.queryByText('Pendiente')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Día por día' }));
  const headers = await screen.findAllByRole('columnheader');
  for (const h of headers) expect(h.textContent?.trim()).not.toBe('');
});

test('sin red, cambiar el periodo quita el reporte viejo y dice «Sin conexión»', async () => {
  renderView();
  await screen.findAllByRole('cell', { name: 'Ana' });
  onlineManager.setOnline(false);
  try {
    report.mockRejectedValue(new ApiError(0, 'NETWORK', 'x', 'r1'));
    fireEvent.click(screen.getByRole('button', { name: 'Semana pasada' }));
    expect(await screen.findByText('Sin conexión', {}, { timeout: 3000 })).toBeInTheDocument();
    expect(screen.queryAllByRole('cell', { name: 'Ana' })).toHaveLength(0);
  } finally {
    onlineManager.setOnline(true);
  }
});

// Al corregir se dice cuál registro, y un traslape dice qué hacer, no el texto crudo del servidor.
test('corregir nombra el registro y explica el traslape', async () => {
  correct.mockRejectedValueOnce(new ApiError(409, 'ATTENDANCE_OVERLAP', 'ese horario se encima con otro registro de esta persona', 'r1'));
  renderView();
  fireEvent.click(await screen.findByRole('button', { name: 'Día por día' }));
  fireEvent.click((await screen.findAllByRole('cell', { name: 'Ana' }))[0]);
  fireEvent.click(await screen.findByRole('button', { name: 'Corregir salida' }));
  expect(screen.getByText('Salida correcta del registro de 09:00 a.m. – 05:00 p.m.')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: /^05:00 p\.m\./ }));
  fireEvent.click(await screen.findByRole('button', { name: '06:00 p.m.' }));
  fireEvent.change(screen.getByPlaceholderText('Motivo (obligatorio)'), { target: { value: 'Se quedó' } });
  fireEvent.click(await screen.findByRole('button', { name: 'Guardar corrección' }));
  expect(await screen.findByText('Esa hora se encima con otro registro de Ana. Revisa los registros de este día.')).toBeInTheDocument();
});

// Corregir arrancaba en «Hora» vacío y la hoja en 12:00 a.m.: había que bajar con el dedo hasta las
// 5 p.m. para mover la salida unos minutos.
test('corregir arranca en la hora que el registro ya tiene', async () => {
  renderView();
  fireEvent.click(await screen.findByRole('button', { name: 'Día por día' }));
  fireEvent.click((await screen.findAllByRole('cell', { name: 'Ana' }))[0]);
  fireEvent.click(await screen.findByRole('button', { name: 'Corregir salida' }));
  expect(await screen.findByRole('button', { name: /05:00 p\.m\./ })).toBeInTheDocument();
});
