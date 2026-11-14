import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { backofficeApi, type CashSessionDetail } from '../../api/backoffice';
import { CorteDetail } from './reporte';

function corte(over: Partial<CashSessionDetail>): CashSessionDetail {
  return {
    id: 410, registerName: 'Caja B', status: 'cerrada', openingCash: '500', currency: 'MXN',
    openedAt: '2026-11-12T14:10:00Z', closedAt: '2026-11-13T04:41:00Z', openedByName: 'Luis', closedByName: 'Luis',
    notes: null, totals: [], movements: [], expenses: [],
    breakdown: { ingresos: [], ingresosTotal: '0', egresos: [], egresosTotal: '0', plataformas: [] } as never,
    sales: [], salesCount: 0, salesShown: 0, salesTotal: '0', counts: null, drawer: null, ...over,
  };
}

function pinta(c: CashSessionDetail, onAbrirCorte = vi.fn()) {
  vi.spyOn(backofficeApi, 'cashSession').mockResolvedValue(c);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <Provider><CorteDetail id={c.id} onAbrirCorte={onAbrirCorte} /></Provider>
    </QueryClientProvider>,
  );
  return onAbrirCorte;
}

afterEach(() => vi.restoreAllMocks());

// EL CASO DE LA CAJA B (#290): el cajero explicó al abrir el turno siguiente qué había pasado y esa
// nota no aparecía en ninguna pantalla. El corte que estuvo mal la muestra arriba, con su diferencia.
test('el corte explicado al abrir el turno siguiente lo dice arriba, con el motivo y la nota', async () => {
  const abrir = pinta(corte({
    difference: { drawer: '-120', otherMethods: '0', total: '-120', drawerCovered: true },
    explainedByNext: { sessionId: 412, reason: 'last_count_wrong', reasonNote: 'El gasto salió de esta caja' },
  }));
  expect(await screen.findByText('Diferencia explicada al abrir el turno #412')).toBeInTheDocument();
  expect(screen.getByText('«El cierre se contó mal»')).toBeInTheDocument();
  expect(screen.getByText('El gasto salió de esta caja')).toBeInTheDocument();
  expect(screen.getByLabelText('Diferencia: Total')).toHaveTextContent('Faltante−$120.00');
  await userEvent.click(screen.getByRole('button', { name: 'Ver el turno #412' }));
  expect(abrir).toHaveBeenCalledWith(412);
});

// El detalle cuenta la historia del turno (FR-006): quién, el fondo que se dejó, el retiro y quién se
// lo llevó, y el motivo con el que abrió.
test('el turno dice quién abrió y cerró, el fondo, el retiro y el motivo de apertura', async () => {
  pinta(corte({
    floatLeft: '500', fixedFloatAtClose: '500', withdrawal: { amount: '2830', userName: 'Carla' },
    openingReason: 'float_changed', openingReasonNote: 'Se subió el fondo',
  }));
  expect(await screen.findByText('$500.00 (fondo fijo $500.00)')).toBeInTheDocument();
  expect(screen.getByText('$2,830.00 · se lo llevó Carla')).toBeInTheDocument();
  expect(screen.getByText('Cambié el fondo: Se subió el fondo')).toBeInTheDocument();
});

// Arqueo ciego: lo oculto no se pinta como «Cuadra».
test('sin diferencia que mostrar no inventa un resumen', async () => {
  pinta(corte({ difference: null, arqueoHidden: true }));
  expect(await screen.findByText('El turno')).toBeInTheDocument();
  expect(screen.queryByText('Cuadra')).not.toBeInTheDocument();
  expect(screen.queryByText(/explicada al abrir/)).not.toBeInTheDocument();
});
