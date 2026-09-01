import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, expect, test, beforeEach } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { LockScreen } from './LockScreen';

const opciones = vi.hoisted(() => ({
  current: { pinOnly: false, users: [{ id: 1, name: 'Ana' }, { id: 2, name: 'Luis' }] },
}));
const pinSwitch = vi.hoisted(() => vi.fn());

vi.mock('../../api/pos', () => ({
  posApi: {
    unlockOptions: () => Promise.resolve(opciones.current),
    pinSwitch: (...a: unknown[]) => pinSwitch(...a),
  },
}));

const salir = vi.fn();
vi.mock('../../stores/session', () => ({
  useSessionStore: (sel: (s: unknown) => unknown) => sel({ clear: salir, setSession: vi.fn() }),
}));

function pintar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider><LockScreen onDesbloqueado={vi.fn()} /></Provider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  opciones.current = { pinOnly: false, users: [{ id: 1, name: 'Ana' }, { id: 2, name: 'Luis' }] };
  pinSwitch.mockReset();
  salir.mockReset();
});

// FR-011 y SC-006. Sin este camino, quien olvida su PIN a media noche queda encerrado fuera del
// punto de venta con el local abierto, y no puede esperar a que otra persona llegue.
//
// Va como test y no como revisión visual porque es exactamente la clase de salida que se cae en un
// refactor sin que nadie lo note hasta esa noche.
test('ofrece entrar con usuario y contraseña a quien olvidó su PIN', async () => {
  pintar();
  const salida = await screen.findByRole('button', { name: /usuario y contraseña/i });
  fireEvent.click(salida);
  // Cerrar la sesión es lo que devuelve a la pantalla de login: la salida existe y funciona.
  await waitFor(() => expect(salir).toHaveBeenCalled());
});

// FR-004: por default se elige a la persona y después se teclea el PIN. Elegir primero es lo que
// hace imposible atribuir la venta a quien no fue.
test('por default pide elegir a la persona antes del PIN', async () => {
  pintar();
  expect(await screen.findByRole('button', { name: 'Ana' })).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Luis' })).toBeTruthy();
});

// FR-005 y contrato: con solo-PIN la lista viaja vacía y la pantalla no debe listar a nadie, o el
// modo perdería su única ventaja y expondría la plantilla sin necesidad.
test('con solo-PIN no muestra a nadie', async () => {
  opciones.current = { pinOnly: true, users: [] };
  pintar();
  await screen.findByRole('button', { name: /usuario y contraseña/i });
  expect(screen.queryByRole('button', { name: 'Ana' })).toBeNull();
});
