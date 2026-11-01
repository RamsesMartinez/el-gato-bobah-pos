import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, expect, test, beforeEach } from 'vitest';
import type { ComponentProps } from 'react';
import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import { TabletaBloqueadaContext } from '../../hooks/tabletaBloqueada';
import { ClockPinSheet } from './ClockPinSheet';

const clock = vi.hoisted(() => vi.fn());
vi.mock('../../api/attendance', () => ({ attendanceApi: { clock: (...a: unknown[]) => clock(...a) } }));

beforeEach(() => { clock.mockReset(); });

type Props = ComponentProps<typeof ClockPinSheet>;

function renderSheet(props: Partial<Props> = {}, locked = false) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const all: Props = {
    open: true, person: { id: 12, name: 'Ana' }, kind: 'in', onClose: vi.fn(), onDone: vi.fn(),
    onStateChanged: vi.fn(), ...props,
  };
  const ui = (p: Props, l: boolean) => (
    <QueryClientProvider client={qc}><Provider>
      <TabletaBloqueadaContext.Provider value={l}><ClockPinSheet {...p} /></TabletaBloqueadaContext.Provider>
    </Provider></QueryClientProvider>
  );
  const r = render(ui(all, locked));
  return { ...r, props: all, rerenderWith: (p: Partial<Props>, l = locked) => r.rerender(ui({ ...all, ...p }, l)) };
}

const typePin = (pin: string) => { for (const d of pin) fireEvent.click(screen.getByRole('button', { name: d })); };
const palette = (name: string) =>
  getComputedStyle(screen.getByRole('button', { name })).getPropertyValue('--chakra-colors-color-palette-solid');

test('entrada: título con la persona y el botón verde', async () => {
  renderSheet({ kind: 'in' });
  expect(await screen.findByText('Marcar entrada · Ana')).toBeInTheDocument();
  expect(screen.getByText('Teclea el PIN de Ana.')).toBeInTheDocument();
  expect(palette('Entrada')).toContain('green');
});

test('salida: título con la persona y el botón rojo, como en Asistencia', async () => {
  renderSheet({ kind: 'out' });
  expect(await screen.findByText('Marcar salida · Ana')).toBeInTheDocument();
  expect(palette('Salida')).toContain('red');
});

test('marca con el mismo endpoint de Asistencia y avisa a quien la abrió', async () => {
  clock.mockResolvedValue({ kind: 'in', at: '2026-10-30T15:02:00Z' });
  const { props } = renderSheet({ kind: 'in' });
  await screen.findByText('Marcar entrada · Ana');
  typePin('4821');
  fireEvent.click(screen.getByRole('button', { name: 'Entrada' }));
  await waitFor(() => expect(props.onDone).toHaveBeenCalledWith({ kind: 'in', at: '2026-10-30T15:02:00Z' }));
  expect(clock).toHaveBeenCalledWith(12, '4821', 'in');
});

test('el error se dice con los mismos textos que Asistencia y limpia el PIN', async () => {
  clock.mockRejectedValue(new ApiError(429, 'TOO_MANY_REQUESTS', 'x', 'r', undefined, 120));
  renderSheet();
  await screen.findByText('Marcar entrada · Ana');
  typePin('1111');
  fireEvent.click(screen.getByRole('button', { name: 'Entrada' }));
  const aviso = await screen.findByText('Demasiados intentos. Intenta de nuevo en 2 min.');
  // Arriba del teclado: debajo quedaba fuera de los 600 px.
  expect(aviso.compareDocumentPosition(screen.getByRole('button', { name: '1' })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(screen.queryByText('••••')).not.toBeInTheDocument();
});

test('si otra tableta ya marcó, la hoja no insiste: se cierra y la caja se vuelve a preguntar', async () => {
  clock.mockRejectedValue(new ApiError(409, 'ATTENDANCE_STATE_CHANGED', 'x', 'r'));
  const { props } = renderSheet();
  await screen.findByText('Marcar entrada · Ana');
  typePin('4821');
  fireEvent.click(screen.getByRole('button', { name: 'Entrada' }));
  await waitFor(() => expect(props.onStateChanged).toHaveBeenCalled());
});

// El teclado físico: con la hoja cerrada o la tableta bloqueada, un dígito no cae en este PIN.
test('el teclado físico solo escribe con la hoja abierta y la tableta sin bloquear', async () => {
  const { rerenderWith } = renderSheet({}, true);
  await screen.findByText('Marcar entrada · Ana');
  fireEvent.keyDown(window, { key: '4' });
  expect(screen.queryByText('•')).not.toBeInTheDocument();
  rerenderWith({}, false);
  fireEvent.keyDown(window, { key: '4' });
  expect(await screen.findByText('•')).toBeInTheDocument();
});

test('el PIN se limpia al bloquearse la tableta y al cerrar', async () => {
  const { rerenderWith } = renderSheet();
  await screen.findByText('Marcar entrada · Ana');
  typePin('48');
  expect(screen.getByText('••')).toBeInTheDocument();
  rerenderWith({}, true);
  await waitFor(() => expect(screen.queryByText('••')).not.toBeInTheDocument());
  rerenderWith({}, false);
  typePin('48');
  rerenderWith({ open: false });
  rerenderWith({ open: true });
  await screen.findByText('Marcar entrada · Ana');
  expect(screen.queryByText('••')).not.toBeInTheDocument();
});

// B14: lo que se marcó ya es un registro de asistencia; cancelar después no deshace nada ni toca la
// caja (la hoja no conoce la caja).
test('cerrar la hoja después de marcar no llama nada más', async () => {
  clock.mockResolvedValue({ kind: 'in', at: '2026-10-30T15:02:00Z' });
  const { props } = renderSheet();
  await screen.findByText('Marcar entrada · Ana');
  typePin('4821');
  fireEvent.click(screen.getByRole('button', { name: 'Entrada' }));
  await waitFor(() => expect(props.onDone).toHaveBeenCalled());
  fireEvent.click(screen.getByRole('button', { name: 'Cerrar' }));
  expect(props.onClose).toHaveBeenCalled();
  expect(clock).toHaveBeenCalledTimes(1);
});
