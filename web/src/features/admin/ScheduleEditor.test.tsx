import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, expect, test, beforeEach } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { ScheduleEditor } from './ScheduleEditor';

const schedules = vi.hoisted(() => vi.fn());
const setSchedule = vi.hoisted(() => vi.fn(() => Promise.resolve()));
const setTracks = vi.hoisted(() => vi.fn(() => Promise.resolve()));
vi.mock('../../api/attendance', () => ({
  attendanceAdminApi: {
    schedules: (id: number) => schedules(id),
    setSchedule: (...a: unknown[]) => setSchedule(...(a as [])),
    setTracks: (...a: unknown[]) => setTracks(...(a as [])),
  },
}));

beforeEach(() => {
  schedules.mockReset().mockImplementation((id: number) => Promise.resolve(id === 2
    ? { tracksAttendance: true, versions: [{ effectiveFrom: '2026-10-01', days: [{ weekday: 5, shiftNo: 1, start: 540, end: 1020 }, { weekday: 5, shiftNo: 2, start: 1320, end: 360 }] }] }
    : { tracksAttendance: true, versions: [] }));
  setSchedule.mockClear();
  setTracks.mockClear();
});

function renderView() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider>
    <ScheduleEditor userId={1} name="Ana" people={[{ id: 1, name: 'Ana' }, { id: 2, name: 'Beto' }]} onClose={vi.fn()} />
  </Provider></QueryClientProvider>);
}

test('copiar el horario de otra persona y guardar crea una versión propia, con su turno nocturno', async () => {
  renderView();
  await screen.findByText('Lunes');
  fireEvent.click(screen.getByText('Copiar de otra persona'));
  fireEvent.click(await screen.findByText('Beto'));
  expect(await screen.findByText('+1 día')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
  await waitFor(() => expect(setSchedule).toHaveBeenCalledTimes(1));
  const [userId, , days] = setSchedule.mock.calls[0] as unknown as [number, string, unknown[]];
  expect(userId).toBe(1);
  expect(days).toEqual([
    { weekday: 5, shiftNo: 1, start: 540, end: 1020 },
    { weekday: 5, shiftNo: 2, start: 1320, end: 360 },
  ]);
  expect(setTracks).not.toHaveBeenCalled();
});

test('sin cambios no se puede guardar', async () => {
  renderView();
  await screen.findByText('Lunes');
  expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
});

// Spec 046: una cuenta que comparten varias personas para abrir caja tiene que llevar «Lleva
// asistencia» apagado. El aviso va detrás del icono de ayuda, como instrucción, no a la vista.
test('la ayuda de «Lleva asistencia» dice qué hacer con una cuenta compartida', async () => {
  renderView();
  expect(screen.queryByText(/misma cuenta/)).not.toBeInTheDocument();
  const ayuda = await screen.findByRole('button', { name: 'Quién lleva asistencia' });
  expect(ayuda).toHaveStyle({ minHeight: '44px' });
  fireEvent.click(ayuda);
  expect(await screen.findByText(/Si varias personas abren la caja con la misma cuenta, apágale «Lleva asistencia» a esa cuenta/)).toBeInTheDocument();
});
