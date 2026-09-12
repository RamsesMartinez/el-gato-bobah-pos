import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, expect, test, beforeEach } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { LockScreen } from './LockScreen';
import { BloqueoPorInactividad } from './BloqueoPorInactividad';

const opciones = vi.hoisted(() => ({
  current: { pinOnly: false, users: [{ id: 1, name: 'Ana' }, { id: 2, name: 'Luis' }] },
}));
const pinSwitch = vi.hoisted(() => vi.fn());
const logout = vi.hoisted(() => vi.fn(() => Promise.resolve()));

vi.mock('../../api/pos', () => ({
  posApi: {
    unlockOptions: () => Promise.resolve(opciones.current),
    pinSwitch: (...a: unknown[]) => pinSwitch(...a),
    logout: () => logout(),
  },
}));

// El bloqueo se fuerza en vez de esperar al cronómetro: lo que este archivo prueba del envoltorio
// es el CABLEADO —que estando bloqueado lo de abajo queda inerte—, no cuándo decide bloquear, que
// tiene sus propios tests en useInactividad.
vi.mock('./useInactividad', () => ({
  useInactividad: () => ({ bloqueado: true, desbloquear: vi.fn() }),
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
  pinSwitch.mockResolvedValue({ accessToken: 'tok', user: { id: 1, name: 'Ana' } });
  salir.mockReset();
  logout.mockClear();
});

// Teclea el PIN con el teclado FÍSICO y devuelve los puntos que quedaron en pantalla.
async function elegirYTeclear(quien: string, teclas: string[]) {
  fireEvent.click(await screen.findByRole('button', { name: quien }));
  for (const k of teclas) fireEvent.keyDown(window, { key: k });
}

// Un "no llamó al servidor" se afirma DESPUÉS de darle su turno a la mutación. Sin esta espera el
// test pasa por llegar antes que la llamada, no por haberla impedido: se comprobó quitando la
// guarda y el test seguía verde.
async function noIntentoEntrar() {
  await new Promise((r) => setTimeout(r, 0));
  expect(pinSwitch).not.toHaveBeenCalled();
}

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
  // Y REVOCA la cookie en el servidor. Con solo limpiar en memoria, una recarga entre el tap y el
  // login canjea la cookie viva y la tableta vuelve sola a la sesión de quien se estaba yendo —
  // justo en la pantalla que existe para quien no puede entrar de otra forma.
  expect(logout).toHaveBeenCalled();
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


// EL TECLADO FÍSICO TECLEA EL PIN.
//
// Varias tabletas del local trabajan con teclado conectado, y el PIN solo se podía marcar con el
// dedo: quien tenía las manos en el teclado tenía que soltarlo y apuntarle a la pantalla.
test('los números del teclado marcan el PIN y Enter entra', async () => {
  pintar();
  await elegirYTeclear('Ana', ['1', '2', '3', '4', 'Enter']);
  await waitFor(() => expect(pinSwitch).toHaveBeenCalledWith(1, '1234'));
});

test('Backspace borra el último dígito', async () => {
  pintar();
  await elegirYTeclear('Ana', ['1', '2', '3', '9', 'Backspace', '4', 'Enter']);
  await waitFor(() => expect(pinSwitch).toHaveBeenCalledWith(1, '1234'));
});

// UN ENTER ANTICIPADO NO PUEDE GASTAR UN INTENTO.
//
// El servidor bloquea la cuenta tras varios fallos seguidos, así que mandar un PIN a medias no es
// inofensivo: acerca al operador al lockout por una tecla de más. El botón ya está apagado con el
// PIN corto y el teclado tiene que respetar la misma condición.
test('Enter con el PIN corto no intenta entrar', async () => {
  pintar();
  await elegirYTeclear('Ana', ['1', '2', 'Enter']);
  await noIntentoEntrar();
});

// Antes de elegir persona no hay PIN que llenar. Acumular dígitos ahí los mandaría con la persona
// que se elija DESPUÉS, y en esta pantalla eso es atribuirle una venta a quien no fue.
test('antes de elegir a la persona el teclado no acumula nada', async () => {
  pintar();
  const ana = await screen.findByRole('button', { name: 'Ana' });
  for (const k of ['1', '2', '3', '4']) fireEvent.keyDown(window, { key: k });
  fireEvent.click(ana);
  expect(screen.getByRole('button', { name: 'Entrar' })).toBeDisabled();
});

// Ctrl+1 cambia de pestaña y Alt+F4 cierra: una combinación no es teclear un PIN, y tomarla como
// dígito deja al operador con un PIN que no escribió y un intento fallido.
test('una combinación con Ctrl no teclea', async () => {
  pintar();
  fireEvent.click(await screen.findByRole('button', { name: 'Ana' }));
  fireEvent.keyDown(window, { key: '1', ctrlKey: true });
  expect(screen.queryByText('•')).toBeNull();
});

// CON EL FOCO EN UN BOTÓN, ENTER ES DE ESE BOTÓN.
//
// Es como se sale por "Entrar con usuario y contraseña" sin ratón — la única salida de quien olvidó
// su PIN a media noche. Robarle la tecla al botón enfocado la dejaría sin camino de teclado y, peor,
// mandaría el PIN cuando el operador creía estar activando otra cosa.
test('Enter con el foco en un botón no manda el PIN', async () => {
  pintar();
  await elegirYTeclear('Ana', ['1', '2', '3', '4']);
  screen.getByRole('button', { name: /usuario y contraseña/i }).focus();
  fireEvent.keyDown(window, { key: 'Enter' });
  await noIntentoEntrar();
});

// EL BLOQUEO ES UN MODAL, y decirlo tiene dos consecuencias.
//
// La primera es de accesibilidad: cubre toda la pantalla y no deja usar nada de abajo.
// La segunda es la que se rompe en silencio: el medidor de toques (spec 019) excluye lo que cae
// dentro de un `[role="dialog"]`, y el bloqueo no cambia de ruta. Sin este atributo, cada
// desbloqueo suma cuatro toques a la pantalla de abajo SIEMPRE EN EL MISMO SITIO —el teclado del
// PIN está centrado— y la rejilla acaba mostrando una zona caliente que nadie tocó ahí.
test('se anuncia como modal, y por eso sus toques no se cuentan en la pantalla de abajo', () => {
  pintar();
  const modal = document.querySelector('[role="dialog"]');
  expect(modal, 'sin role="dialog" el teclado del PIN contamina la rejilla de la pantalla de abajo').not.toBeNull();
  expect(modal!.getAttribute('aria-modal')).toBe('true');
});

// Y LA PROMESA DE `aria-modal` LA TIENE QUE CUMPLIR ALGUIEN.
//
// El atributo dice «lo de abajo no se puede usar». Sin `inert`, con un teclado —las tabletas del
// local a veces traen uno— `Tab` desde el último botón del bloqueo cae en un control invisible de
// la pantalla de abajo y `Enter` lo activa: usar el POS sin desbloquearlo, que es justo lo que esta
// pantalla existe para impedir.
test('mientras bloquea, lo de abajo queda fuera del alcance del dedo y del tabulador', () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <Provider>
        <BloqueoPorInactividad>
          <button type="button">Cobrar</button>
        </BloqueoPorInactividad>
      </Provider>
    </QueryClientProvider>,
  );
  const envoltura = screen.getByRole('button', { name: 'Cobrar' }).closest('[inert]');
  expect(envoltura, 'sin inert, aria-modal es una etiqueta que el bloqueo no cumple').not.toBeNull();
});
