import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query';
import { vi, expect, test, beforeEach } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import { ClockView } from './ClockView';

const roster = vi.hoisted(() => vi.fn());
const clock = vi.hoisted(() => vi.fn());
vi.mock('../../api/attendance', () => ({ attendanceApi: { roster: () => roster(), clock: (...a: unknown[]) => clock(...a) } }));
vi.mock('../../hooks/useHoraDelNegocio', () => ({ useHoraDelNegocio: () => ({ zona: 'America/Mexico_City' }) }));

const sampleRoster = {
  enabled: true,
  people: [
    { userId: 1, name: 'Ana', openSince: '2026-10-10T15:02:00Z', hasPin: true },
    { userId: 4, name: 'Luz', leftAt: '2026-10-10T20:30:00Z', hasPin: true },
    { userId: 2, name: 'Beto', hasPin: true },
    { userId: 3, name: 'Mar', hasPin: false },
  ],
};

beforeEach(() => {
  roster.mockReset().mockResolvedValue(sampleRoster);
  clock.mockReset();
});

function renderView() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><ClockView /></Provider></QueryClientProvider>);
}

const typeDigit = (pin: string) => { for (const d of pin) fireEvent.click(screen.getByRole('button', { name: d })); };

test('reparte a la gente entre «Por entrar» y «Dentro»', async () => {
  renderView();
  expect(await screen.findByText('Dentro desde 09:02 a.m.')).toBeInTheDocument();
  expect(screen.getByText('1 en el local')).toBeInTheDocument();
  expect(screen.getByText('Sin entrada hoy')).toBeInTheDocument();
});

test('quien no tiene PIN no puede tocar su nombre y se le dice qué hacer', async () => {
  renderView();
  const mar = (await screen.findByText('Aún no tiene PIN')).closest('button');
  expect(mar).toBeDisabled();
});

test('quien está dentro ve «Salida» y quien no, «Entrada»', async () => {
  renderView();
  fireEvent.click(await screen.findByText('Ana'));
  expect(await screen.findByRole('button', { name: 'Salida' })).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'No soy yo' }));
  fireEvent.click(screen.getByText('Beto'));
  expect(await screen.findByRole('button', { name: 'Entrada' })).toBeInTheDocument();
});

test('al abrir el teclado se vuelve a pedir la lista: otra tableta pudo checar a esa persona', async () => {
  renderView();
  await screen.findByText('Beto');
  roster.mockResolvedValue({ ...sampleRoster, people: sampleRoster.people.map((p) => (p.userId === 2 ? { ...p, openSince: '2026-10-10T15:30:00Z' } : p)) });
  fireEvent.click(screen.getByText('Beto'));
  expect(await screen.findByRole('button', { name: 'Salida' })).toBeInTheDocument();
});

test('un PIN equivocado no dice si falló la persona o el PIN', async () => {
  clock.mockRejectedValue(new ApiError(401, 'INVALID_CREDENTIALS', 'x', 'id'));
  renderView();
  fireEvent.click(await screen.findByText('Beto'));
  await screen.findByRole('button', { name: 'Entrada' });
  typeDigit('1234');
  fireEvent.click(screen.getByRole('button', { name: 'Entrada' }));
  expect(await screen.findByText('No coincide. Inténtalo de nuevo.')).toBeInTheDocument();
});

test('con el bloqueo por intentos dice en cuántos minutos reintentar', async () => {
  clock.mockRejectedValue(new ApiError(429, 'TOO_MANY_REQUESTS', 'x', 'id', undefined, 300));
  renderView();
  fireEvent.click(await screen.findByText('Beto'));
  await screen.findByRole('button', { name: 'Entrada' });
  typeDigit('1234');
  fireEvent.click(screen.getByRole('button', { name: 'Entrada' }));
  expect(await screen.findByText('Demasiados intentos. Intenta de nuevo en 5 min.')).toBeInTheDocument();
});

test('al checar confirma con la hora del servidor', async () => {
  clock.mockResolvedValue({ kind: 'out', at: '2026-10-11T00:12:00Z', workedMinutes: 490 });
  renderView();
  fireEvent.click(await screen.findByText('Ana'));
  await screen.findByRole('button', { name: 'Salida' });
  typeDigit('1234');
  fireEvent.click(screen.getByRole('button', { name: 'Salida' }));
  expect(await screen.findByText('Salida 06:12 p.m.')).toBeInTheDocument();
  expect(screen.getByText('Trabajaste 8 h 10 min')).toBeInTheDocument();
  expect(clock).toHaveBeenCalledWith(1, '1234', 'out');
});

test('Enter del teclado físico manda un solo intento', async () => {
  clock.mockResolvedValue({ kind: 'in', at: '2026-10-10T15:00:00Z' });
  renderView();
  fireEvent.click(await screen.findByText('Beto'));
  await screen.findByRole('button', { name: 'Entrada' });
  for (const d of '1234') fireEvent.keyDown(window, { key: d });
  fireEvent.keyDown(window, { key: 'Enter' });
  await waitFor(() => expect(clock).toHaveBeenCalledTimes(1));
});

// Edge case «Sin red»: una lista vacía sin explicación se lee como «no hay nadie que checar».
test('sin red dice «Sin conexión» y deja reintentar', async () => {
  roster.mockRejectedValue(new ApiError(0, 'NETWORK', 'Sin conexión con el servidor', 'r0'));
  renderView();
  expect(await screen.findByText('Sin conexión', {}, { timeout: 3000 })).toBeInTheDocument();
  roster.mockResolvedValue(sampleRoster);
  fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }));
  expect(await screen.findByText('Ana')).toBeInTheDocument();
  expect(screen.queryByText('Sin conexión')).toBeNull();
});

// Con red pero con el servidor fallando, mandar a revisar el internet es mandar a buscar donde no es.
test('si responde el servidor con error no dice «Sin conexión»', async () => {
  roster.mockRejectedValue(new ApiError(500, 'INTERNAL', 'falló', 'r1'));
  renderView();
  expect(await screen.findByText('No se pudo cargar', {}, { timeout: 3000 })).toBeInTheDocument();
  expect(screen.queryByText('Sin conexión')).toBeNull();
  expect(screen.getByRole('button', { name: 'Reintentar' })).toBeInTheDocument();
});

// La red caída llega del cliente como ApiError NETWORK, no como TypeError: es el caso real.
test('la red caída del cliente también dice «Sin conexión»', async () => {
  roster.mockRejectedValue(new ApiError(0, 'NETWORK', 'Sin conexión con el servidor', 'r1'));
  renderView();
  expect(await screen.findByText('Sin conexión', {}, { timeout: 3000 })).toBeInTheDocument();
});

// SIN RED, LA LISTA VIEJA NO SE QUEDA COMO SI FUERA CIERTA. TanStack pausaba la consulta con el
// navegador desconectado: ni error ni aviso, y «1 en el local» seguía en pantalla.
test('si se cae la red con la lista ya pintada, la lista se quita y dice «Sin conexión»', async () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><ClockView /></Provider></QueryClientProvider>);
  await screen.findByText('Ana');
  onlineManager.setOnline(false);
  try {
    roster.mockRejectedValue(new ApiError(0, 'NETWORK', 'x', 'r2'));
    await qc.refetchQueries({ queryKey: ['asistencia', 'roster'] });
    expect(await screen.findByText('Sin conexión', {}, { timeout: 3000 })).toBeInTheDocument();
    expect(screen.queryByText('1 en el local')).toBeNull();
    expect(screen.queryByText('Ana')).toBeNull();
  } finally {
    onlineManager.setOnline(true);
  }
});

// Quien ya salió hoy no se ve «Sin entrada hoy»: se leía como que su día no quedó registrado.
test('quien ya salió hoy dice a qué hora salió', async () => {
  renderView();
  expect(await screen.findByText('Salió a las 02:30 p.m.')).toBeInTheDocument();
});

// Un TypeError del código no es la red: decir «revisa el internet» manda a buscar donde no es.
test('un error del código no se anuncia como «Sin conexión»', async () => {
  roster.mockRejectedValue(new TypeError("Cannot read properties of undefined (reading 'map')"));
  renderView();
  expect(await screen.findByText('No se pudo cargar', {}, { timeout: 3000 })).toBeInTheDocument();
  expect(screen.queryByText('Sin conexión')).toBeNull();
});

// Sin red la lista no se vuelve a pedir sola: sin escuchar al navegador, la lista vieja se quedaba.
test('si el navegador pierde la red, quita la lista aunque nada se haya vuelto a pedir', async () => {
  renderView();
  await screen.findByText('Ana');
  const spy = vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false);
  try {
    window.dispatchEvent(new Event('offline'));
    expect(await screen.findByText('Sin conexión')).toBeInTheDocument();
    expect(screen.queryByText('Ana')).toBeNull();
  } finally {
    spy.mockRestore();
    window.dispatchEvent(new Event('online'));
  }
});
