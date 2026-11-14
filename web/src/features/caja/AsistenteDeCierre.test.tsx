import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { Provider } from '../../components/ui/provider';
import { backofficeApi, type CashSession } from '../../api/backoffice';
import { useSessionStore } from '../../stores/session';
import { CashPage } from './CashPage';

function Donde() {
  return <span data-testid="ruta">{useLocation().pathname}</span>;
}

const caja = { id: 4, name: 'Caja principal', isPrimary: true, isActive: true, openSessionId: 60, countsCash: true };

function turno(over: Partial<CashSession> = {}): CashSession {
  return {
    id: 60, registerId: 4, registerName: 'Caja principal', isPrimary: true, status: 'abierta', openingCash: '500',
    currency: 'MXN', openedAt: '2026-11-13T14:00:00Z', netMovements: '0',
    totals: [
      { methodId: 1, name: 'Efectivo', kind: 'efectivo', expected: '835', declared: '0', difference: '0', autoDeclare: false, requiresEntry: false },
      { methodId: 2, name: 'Tarjeta', kind: 'tarjeta', expected: '300', declared: '0', difference: '0', autoDeclare: false, requiresEntry: true },
    ],
    movements: [], expenses: [],
    breakdown: { ingresos: [], ingresosTotal: '0', egresos: [], egresosTotal: '0', plataformas: [] } as never,
    pending: [{ number: 12, name: 'Mesa 4', id: 99, total: '185' }] as never, cashiers: [],
    uncollected: '0', uncollectedCount: 0, counts: null,
    drawer: { expected: '835', counted: null, difference: null, methodIds: [1], requiresCount: true },
    fixedFloat: '500', tipsPending: { total: '0', cash: '0', nonCash: '0', inherited: '0', people: [{ id: 1, name: 'Ana' }] },
    ...over,
  };
}

function pinta(ruta: string, sesion: CashSession | null | 'sin-red' = turno(), permissions = ['cash.operate', 'cash.configure']) {
  useSessionStore.setState({ user: { id: 1, companyId: 1, name: 'Ana', role: 'gerente', permissions } as never });
  vi.spyOn(backofficeApi, 'cashRegisters').mockResolvedValue({ items: [{ ...caja, openSessionId: sesion ? 60 : null }] } as never);
  if (sesion === 'sin-red') vi.spyOn(backofficeApi, 'cashCurrent').mockRejectedValue(new Error('sin red'));
  else vi.spyOn(backofficeApi, 'cashCurrent').mockResolvedValue(sesion as never);
  vi.spyOn(backofficeApi, 'cashAttendance').mockResolvedValue({ stage: 'close', applies: false, ready: true } as never);
  vi.spyOn(backofficeApi, 'cashCloseSettings').mockResolvedValue({ tolerance: '0' });
  vi.spyOn(backofficeApi, 'cashHistory').mockResolvedValue({ items: [], total: 0 } as never);
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <Provider><MemoryRouter initialEntries={[ruta]}><Routes><Route path="/caja/*" element={<><CashPage /><Donde /></>} /></Routes></MemoryRouter></Provider>
    </QueryClientProvider>,
  );
}

beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());

// «CÓMO VA EL TURNO» (Corte X) va al lado de Operar y solo consulta: «Cerrar caja» lleva al asistente.
test('el turno abierto muestra «Cómo va el turno» y Cerrar caja abre el asistente', async () => {
  pinta('/caja/4');
  expect(await screen.findByText('Cómo va el turno')).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: /Cerrar caja/ }));
  expect(await screen.findByRole('navigation', { name: 'Pasos del cierre' })).toBeInTheDocument();
});

// LA LISTA Y EL BOTÓN SALEN DE LA MISMA REGLA (FR-030): en el último paso, cada pendiente lleva a su
// paso y el botón no se habilita con alguno sin resolver.
test('el último paso dice qué falta y no deja cerrar', async () => {
  pinta('/caja/4/cerrar');
  const pasos = await screen.findByRole('navigation', { name: 'Pasos del cierre' });
  expect(pasos).toHaveTextContent('1 pendiente'); // el pedido sin entregar
  await userEvent.click(screen.getByRole('button', { name: /Diferencia y cierre/ }));
  expect(screen.getByText('1 pedido sin entregar')).toBeInTheDocument();
  expect(screen.getByText('Contar el efectivo del cajón')).toBeInTheDocument();
  expect(screen.getByText('Escribir lo contado de Tarjeta')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Cerrar caja' })).toBeDisabled();
  await userEvent.click(screen.getAllByRole('button', { name: 'Ir a Otros métodos' })[0]);
  expect(await screen.findByText('Cierre — contado por método')).toBeInTheDocument();
});

// EL BORRADOR SOBREVIVE A UNA RECARGA (FR-032): lo tecleado en el conteo de otros métodos sigue ahí.
test('lo capturado sigue ahí después de recargar', async () => {
  const { unmount } = pinta('/caja/4/cerrar');
  await userEvent.click(await screen.findByRole('button', { name: /Otros métodos/ }));
  await userEvent.type(screen.getByLabelText('Declarado de Tarjeta'), '300');
  unmount();
  pinta('/caja/4/cerrar');
  expect(await screen.findByLabelText('Declarado de Tarjeta')).toHaveValue('300');
});

// Un borrador de un turno que otra tableta ya cerró se descarta, y se dice.
test('el borrador de una caja que ya se cerró se descarta', async () => {
  localStorage.setItem('egb:cierre:4:60', JSON.stringify({ declared: { 2: '300' } }));
  pinta('/caja/4', null);
  await waitFor(() => expect(localStorage.getItem('egb:cierre:4:60')).toBeNull());
});

// Con arqueo ciego el último paso no muestra diferencia ni pide explicación: así no se puede recontar
// hasta cuadrar (decisión P1).
test('con arqueo ciego no se pide explicación', async () => {
  pinta('/caja/4/cerrar', turno({ blind: true, drawer: { expected: null, counted: null, difference: null, methodIds: [1], requiresCount: true } }), ['cash.operate']);
  await userEvent.click(await screen.findByRole('button', { name: /Diferencia y cierre/ }));
  expect(screen.queryByText(/Explica la diferencia/)).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Diferencia del arqueo')).not.toBeInTheDocument();
});

// Con quien abrió todavía dentro, el aviso queda en el primer paso y el cierre pendiente, pero el
// conteo no se esconde: se cuenta mientras se resuelve (FR-031). Antes el aviso tapaba todo el cierre.
test('el aviso de asistencia no esconde el conteo', async () => {
  pinta('/caja/4/cerrar', turno({ pending: [] as never }));
  vi.mocked(backofficeApi.cashAttendance).mockResolvedValue({
    stage: 'close', applies: true, ready: false, person: { id: 12, name: 'Beto', active: true },
    stale: false, isSelf: false, canAuthorize: false, openSince: '2026-11-13T14:00:00Z',
  } as never);
  expect(await screen.findByText('Beto abrió esta caja y no ha marcado su salida')).toBeInTheDocument();
  await userEvent.click(within(screen.getByRole('navigation', { name: 'Pasos del cierre' })).getByRole('button', { name: /Cajón/ }));
  expect(screen.getByRole('button', { name: 'Contar efectivo' })).toBeEnabled();
  await userEvent.click(screen.getByRole('button', { name: /Diferencia y cierre/ }));
  expect(screen.getByRole('button', { name: 'Cerrar caja' })).toBeDisabled();
});

// LO QUE ENCONTRÓ LA REVISIÓN DE LA FASE 4 EN EL BORRADOR Y LA RUTA DEL CIERRE.

// Sin red al recargar, la consulta del turno falla: eso no es «la caja ya se cerró». Se borraba el
// borrador y se avisaba de otra tableta en el único caso que el borrador venía a cubrir.
test('si la red falla al cargar, el borrador se queda y no se avisa de otra tableta', async () => {
  localStorage.setItem('egb:cierre:4:60', JSON.stringify({ declared: { 2: '300' } }));
  pinta('/caja/4/cerrar', 'sin-red');
  await waitFor(() => expect(backofficeApi.cashCurrent).toHaveBeenCalled());
  await new Promise((r) => setTimeout(r, 50));
  expect(localStorage.getItem('egb:cierre:4:60')).not.toBeNull();
});

// Mirar el turno no es capturar: una tableta que solo miraba la caja recibía «Se descartó lo que se
// había capturado aquí» cuando otra la cerraba.
test('mirar el turno sin capturar no deja borrador', async () => {
  pinta('/caja/4');
  expect(await screen.findByText('Cómo va el turno')).toBeInTheDocument();
  await new Promise((r) => setTimeout(r, 50));
  expect(localStorage.getItem('egb:cierre:4:60')).toBeNull();
});

// El borrador de un turno anterior de esta caja se va sin avisar al montar el turno nuevo: si se
// quedaba, el aviso de «otra tableta» salía después en un cierre propio.
test('el borrador de un turno anterior se borra al montar el actual', async () => {
  localStorage.setItem('egb:cierre:4:59', JSON.stringify({ declared: { 2: '300' } }));
  pinta('/caja/4');
  expect(await screen.findByText('Cómo va el turno')).toBeInTheDocument();
  await waitFor(() => expect(localStorage.getItem('egb:cierre:4:59')).toBeNull());
});

// La dirección del cierre de una caja que ya no tiene turno vuelve a la caja: abrir el turno nuevo
// lo metía directo al asistente de cierre.
test('la dirección del cierre sin turno abierto vuelve a la caja', async () => {
  pinta('/caja/4/cerrar', null);
  await waitFor(() => expect(screen.getByTestId('ruta')).toHaveTextContent(/^\/caja\/4$/));
});

// La explicación: la deja quien ve la diferencia aunque no configure la caja (FR-033), un salto de
// línea no tumba el cierre (el servidor rechaza caracteres de control) y se guarda en el borrador.
function cuadraSalvoTarjeta(): CashSession {
  return turno({
    pending: [] as never, drawer: null as never,
    totals: [{ methodId: 2, name: 'Tarjeta', kind: 'tarjeta', expected: '300', declared: '0', difference: '0', autoDeclare: false, requiresEntry: true }],
  });
}

test('quien cierra viendo la diferencia la explica, en un solo renglón', async () => {
  const cerrar = vi.spyOn(backofficeApi, 'cashClose').mockResolvedValue(turno({ status: 'cerrada' }) as never);
  pinta('/caja/4/cerrar', cuadraSalvoTarjeta(), ['cash.operate']);
  await userEvent.click(await screen.findByRole('button', { name: /Otros métodos/ }));
  await userEvent.type(screen.getByLabelText('Declarado de Tarjeta'), '250');
  await userEvent.click(within(screen.getByRole('navigation', { name: 'Pasos del cierre' })).getByRole('button', { name: /Diferencia y cierre/ }));
  await userEvent.click(screen.getByRole('button', { name: 'Se contó mal' }));
  expect(screen.getByRole('button', { name: 'Cerrar caja' })).toBeDisabled(); // motivo sin nota
  await userEvent.type(screen.getByLabelText('Qué pasó con la diferencia'), 'Contamos mal{Enter}faltaba un billete');
  expect(JSON.parse(localStorage.getItem('egb:cierre:4:60') ?? '{}').explicacion?.reason).toBe('miscount');
  await userEvent.click(screen.getByRole('button', { name: 'Cerrar caja' }));
  await userEvent.click(within(screen.getAllByRole('dialog').at(-1)!).getByRole('button', { name: 'Cerrar caja' }));
  await waitFor(() => expect(cerrar).toHaveBeenCalled());
  expect(cerrar.mock.calls[0][2]?.explanation).toEqual({ reason: 'miscount', note: 'Contamos mal faltaba un billete' });
});
