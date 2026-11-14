import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router';
import { Provider } from '../../components/ui/provider';
import { backofficeApi, type CashSession } from '../../api/backoffice';
import { useSessionStore } from '../../stores/session';
import { CashPage } from './CashPage';

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

function pinta(ruta: string, sesion: CashSession | null = turno(), permissions = ['cash.operate', 'cash.configure']) {
  useSessionStore.setState({ user: { id: 1, companyId: 1, name: 'Ana', role: 'gerente', permissions } as never });
  vi.spyOn(backofficeApi, 'cashRegisters').mockResolvedValue({ items: [{ ...caja, openSessionId: sesion ? 60 : null }] } as never);
  vi.spyOn(backofficeApi, 'cashCurrent').mockResolvedValue(sesion as never);
  vi.spyOn(backofficeApi, 'cashAttendance').mockResolvedValue({ stage: 'close', applies: false, ready: true } as never);
  vi.spyOn(backofficeApi, 'cashCloseSettings').mockResolvedValue({ tolerance: '0' });
  vi.spyOn(backofficeApi, 'cashHistory').mockResolvedValue({ items: [], total: 0 } as never);
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <Provider><MemoryRouter initialEntries={[ruta]}><Routes><Route path="/caja/*" element={<CashPage />} /></Routes></MemoryRouter></Provider>
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
