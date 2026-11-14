import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { backofficeApi, type CashSessionDetail } from '../../api/backoffice';
import { useSessionStore } from '../../stores/session';
import { CorteDetail } from './reporte';

function corte(over: Partial<CashSessionDetail>): CashSessionDetail {
  return {
    id: 410, registerName: 'Caja B', status: 'cerrada', openingCash: '500', currency: 'MXN',
    openedAt: '2026-11-12T14:10:00Z', closedAt: '2026-11-13T04:41:00Z', openedByName: 'Luis', closedByName: 'Luis',
    notes: null, totals: [], movements: [], expenses: [],
    breakdown: { ingresos: [], ingresosTotal: '0', egresos: [], egresosTotal: '0', plataformas: [] } as never,
    sales: [], salesCount: 0, salesShown: 0, salesTotal: '0', counts: null,
    drawer: { expected: '835', counted: '785', difference: '-50', methodIds: [1], requiresCount: false },
    difference: { drawer: '-50', otherMethods: '0', total: '-50', drawerCovered: true },
    adjustments: [], effective: null, ...over,
  };
}

function pinta(c: CashSessionDetail, permissions = ['cash.operate', 'cash.configure', 'cash.adjust_count']) {
  useSessionStore.setState({ user: { id: 1, companyId: 1, name: 'Ana', role: 'gerente', permissions } as never });
  vi.spyOn(backofficeApi, 'cashSession').mockResolvedValue(c);
  render(<QueryClientProvider client={new QueryClient()}><Provider><CorteDetail id={c.id} /></Provider></QueryClientProvider>);
}

afterEach(() => vi.restoreAllMocks());

// CORREGIR UN CORTE CERRADO (#290, fase 3; Maquetas M4): qué corriges, lo que debió decir, el motivo
// y una nota que no puede faltar. Se manda al servidor tal cual; el «antes» lo pone él.
test('corregir lo contado manda tipo, monto, motivo y nota', async () => {
  const ajustar = vi.spyOn(backofficeApi, 'adjustCashSession').mockResolvedValue({} as never);
  pinta(corte({}));
  await userEvent.click(await screen.findByRole('button', { name: 'Corregir' }));
  await userEvent.click(screen.getByRole('button', { name: 'Lo contado en el cajón' }));
  await userEvent.type(screen.getByLabelText('Debió decir'), '835');
  await userEvent.click(screen.getByRole('button', { name: 'Se contó mal' }));
  const guardar = screen.getByRole('button', { name: 'Guardar corrección' });
  expect(guardar).toBeDisabled(); // sin nota no hay rastro de por qué
  await userEvent.type(screen.getByLabelText('Qué pasó'), 'Un billete de $50 estaba en el sobre');
  await userEvent.click(guardar);
  await waitFor(() => expect(ajustar).toHaveBeenCalledWith(410, {
    kind: 'count_correction', after: '835', reason: 'miscount', note: 'Un billete de $50 estaba en el sobre' }));
});

test('una nota no pide monto', async () => {
  const ajustar = vi.spyOn(backofficeApi, 'adjustCashSession').mockResolvedValue({} as never);
  pinta(corte({}));
  await userEvent.click(await screen.findByRole('button', { name: 'Corregir' }));
  await userEvent.click(screen.getByRole('button', { name: 'Solo una nota' }));
  expect(screen.queryByLabelText('Debió decir')).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Otro' }));
  await userEvent.type(screen.getByLabelText('Qué pasó'), 'El gasto salió de esta caja');
  await userEvent.click(screen.getByRole('button', { name: 'Guardar corrección' }));
  await waitFor(() => expect(ajustar).toHaveBeenCalledWith(410, { kind: 'note', reason: 'other', note: 'El gasto salió de esta caja' }));
});

// Lo firmado no se borra: se ve lo contado, lo corregido, quién y por qué, y se puede revertir.
test('el corte corregido muestra el original, la corrección y Revertir', async () => {
  const revertir = vi.spyOn(backofficeApi, 'reverseCashSessionAdjustment').mockResolvedValue({} as never);
  pinta(corte({
    adjustments: [{ id: 7, kind: 'count_correction', before: '785', after: '835', reason: 'miscount', reasonNote: 'Billete en el sobre',
      createdByName: 'Carla', createdAt: '2026-11-13T16:15:00Z', reversesId: null, reversedBy: null, reversible: true }],
    effective: { drawerCounted: '835', floatLeft: null },
  }));
  expect(await screen.findByText('Contado $785.00, corregido a $835.00')).toBeInTheDocument();
  expect(screen.getByText(/Carla · Se contó mal/)).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Revertir' }));
  await userEvent.type(await screen.findByRole('textbox'), 'Me equivoqué de corte');
  await userEvent.click(screen.getByRole('button', { name: 'Revertir la corrección' }));
  await waitFor(() => expect(revertir).toHaveBeenCalledWith(410, 7, 'Me equivoqué de corte'));
});

test('una corrección revertida no ofrece Revertir', async () => {
  pinta(corte({
    adjustments: [
      { id: 7, kind: 'note', before: null, after: null, reason: 'other', reasonNote: 'x', createdByName: 'Carla', createdAt: '2026-11-13T16:15:00Z', reversesId: null, reversedBy: 8 },
      { id: 8, kind: 'reversal', before: null, after: null, reason: 'other', reasonNote: 'y', createdByName: 'Carla', createdAt: '2026-11-13T16:20:00Z', reversesId: 7, reversedBy: null },
    ],
  }));
  expect(await screen.findByText('Revertida')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Revertir' })).not.toBeInTheDocument();
});

// Quien solo opera la caja no ve la acción; la barrera de verdad es el servidor (403).
test('sin permiso de ajustar el arqueo no hay Corregir', async () => {
  pinta(corte({}), ['cash.operate']);
  expect(await screen.findByText('El turno')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Corregir' })).not.toBeInTheDocument();
});

// Cambiar de «qué corriges» borra el monto: un 500 tecleado para lo contado no se guarda como fondo
// (revisión de la fase 3).
test('cambiar lo que se corrige borra el monto tecleado', async () => {
  pinta(corte({}));
  await userEvent.click(await screen.findByRole('button', { name: 'Corregir' }));
  await userEvent.click(screen.getByRole('button', { name: 'Lo contado en el cajón' }));
  await userEvent.type(screen.getByLabelText('Debió decir'), '500');
  await userEvent.click(screen.getByRole('button', { name: 'El fondo que se dejó' }));
  expect(screen.getByLabelText('Debió decir')).toHaveValue('');
});

// «Revertir» solo donde el servidor lo acepta: una corrección de abajo, con otra encima, no se ofrece.
test('una corrección que ya no es la vigente no ofrece Revertir', async () => {
  pinta(corte({
    adjustments: [
      { id: 7, kind: 'count_correction', before: '785', after: '800', reason: 'miscount', reasonNote: 'a', createdByName: 'Carla', createdAt: '2026-11-13T16:15:00Z', reversesId: null, reversedBy: null, reversible: false },
      { id: 8, kind: 'count_correction', before: '800', after: '835', reason: 'miscount', reasonNote: 'b', createdByName: 'Carla', createdAt: '2026-11-13T16:20:00Z', reversesId: null, reversedBy: null, reversible: true },
    ],
  }));
  expect(await screen.findAllByRole('button', { name: 'Revertir' })).toHaveLength(1);
});
