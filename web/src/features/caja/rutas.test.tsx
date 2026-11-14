import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { Provider } from '../../components/ui/provider';
import { backofficeApi } from '../../api/backoffice';
import { useSessionStore } from '../../stores/session';
import { CashPage } from './CashPage';
import { ApiError } from '../../api/client';
import { canOpen } from '../../app/routes';

function Donde() {
  return <span data-testid="ruta">{useLocation().pathname}</span>;
}

const cajas = [
  { id: 4, name: 'Caja principal', isPrimary: true, isActive: true, openSessionId: null, countsCash: true },
  { id: 5, name: 'Caja clip', isPrimary: false, isActive: true, openSessionId: null, countsCash: true },
];

function pinta(ruta: string, permissions: string[] = ['cash.operate', 'cash.configure'], items: unknown[] = [], historia: unknown = { items: [], total: 0 }) {
  useSessionStore.setState({ user: { id: 1, companyId: 1, name: 'Ana', role: 'gerente', permissions } as never });
  vi.spyOn(backofficeApi, 'cashRegisters').mockResolvedValue({ items } as never);
  vi.spyOn(backofficeApi, 'allCashRegisters').mockResolvedValue({ items: [] });
  vi.spyOn(backofficeApi, 'cashHistory').mockResolvedValue(historia as never);
  vi.spyOn(backofficeApi, 'cashSession').mockResolvedValue({ id: 410 } as never);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <Provider>
        <MemoryRouter initialEntries={[ruta]}>
          <Routes><Route path="/caja/*" element={<><CashPage /><Donde /></>} /></Routes>
        </MemoryRouter>
      </Provider>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.restoreAllMocks());

// CADA PESTAÑA Y CADA CORTE TIENEN DIRECCIÓN (#290, FR-017): una recarga vuelve a donde se estaba y
// «corte #410» se puede compartir. Antes la pestaña vivía en memoria y toda recarga caía en «Cajas».
test('la dirección del Histórico abre el Histórico', async () => {
  pinta('/caja/historico');
  expect(await screen.findByRole('tab', { name: 'Histórico', selected: true })).toBeInTheDocument();
});

test('la dirección de un corte abre ese corte', async () => {
  pinta('/caja/historico/410');
  expect(await screen.findByText('Corte #410')).toBeInTheDocument();
});

test('Administrar tiene su dirección', async () => {
  pinta('/caja/administrar');
  expect(await screen.findByRole('tab', { name: 'Administrar', selected: true })).toBeInTheDocument();
});

// Sin permiso de configurar, la dirección de Administrar no muestra nada de configuración: cae en
// Cajas (la barrera de verdad está en el servidor).
test('sin permiso, /caja/administrar cae en Cajas', async () => {
  pinta('/caja/administrar', ['cash.operate']);
  expect(await screen.findByRole('tab', { name: 'Cajas', selected: true })).toBeInTheDocument();
  expect(screen.queryByRole('tab', { name: 'Administrar' })).not.toBeInTheDocument();
});

test('cambiar de pestaña cambia la dirección', async () => {
  pinta('/caja');
  await userEvent.click(await screen.findByRole('tab', { name: 'Histórico' }));
  await waitFor(() => expect(screen.getByTestId('ruta')).toHaveTextContent('/caja/historico'));
});

// Una dirección con un corte que no existe —o de otra empresa— no deja el diálogo cargando para
// siempre (revisión de la fase 2): dice que no lo encontró.
test('un corte inexistente dice que no se encontró', async () => {
  pinta('/caja/historico/99999');
  vi.spyOn(backofficeApi, 'cashSession').mockRejectedValue(new ApiError(404, 'NOT_FOUND', 'no encontrado', 'r1'));
  expect(await screen.findByText('No encontramos ese corte.')).toBeInTheDocument();
});

// Un parámetro presente y malformado no cae a un default en silencio (constitución V): /caja/999
// mostraba la caja principal con la dirección diciendo otra cosa.
test('una caja inexistente lleva a Cajas y avisa', async () => {
  pinta('/caja/999', ['cash.operate'], cajas);
  expect(await screen.findByText('No encontramos esa caja.')).toBeInTheDocument();
  await waitFor(() => expect(screen.getByTestId('ruta')).toHaveTextContent(/^\/caja$/));
});

// Cambiar de pestaña no olvida la caja elegida: volver a «Cajas» con otra caja abierta en pantalla
// es operar la equivocada.
test('volver a Cajas regresa a la caja que estaba elegida', async () => {
  pinta('/caja/5', ['cash.operate'], cajas);
  await userEvent.click(await screen.findByRole('tab', { name: 'Histórico' }));
  await userEvent.click(await screen.findByRole('tab', { name: 'Cajas' }));
  await waitFor(() => expect(screen.getByTestId('ruta')).toHaveTextContent('/caja/5'));
});

// Quien configura el negocio sin operar la caja veía los ajustes de caja en Negocio; ahora viven en
// Caja → Administrar, y esa dirección le tiene que abrir.
test('configurar el negocio sin operar la caja abre Caja → Administrar', () => {
  const user = { id: 1, companyId: 1, name: 'Ana', role: 'propio', permissions: ['business.configure'] } as never;
  expect(canOpen('/caja/administrar', user)).toBe(true);
  expect(canOpen('/caja', user)).toBe(false);
});

// La lista marca los cortes corregidos y suma los sobrantes y faltantes del periodo, que vienen del
// servidor para todo el filtro y no solo de la página (#290, fase 3; P4).
test('el Histórico marca lo corregido y dice la suma del periodo', async () => {
  pinta('/caja/historico', undefined, [], {
    items: [{ id: 410, registerName: 'Caja B', status: 'cerrada', openingCash: '500', currency: 'MXN', openedAt: '2026-11-12T14:10:00Z',
      closedAt: '2026-11-13T04:41:00Z', openedByName: 'Luis', closedByName: 'Luis', totalDifference: '0', notes: null,
      difference: { drawer: '0', otherMethods: '0', total: '0', drawerCovered: true }, adjusted: true }],
    total: 1, page: 0, pageSize: 20, periodDifference: '-100',
  });
  expect(await screen.findByText('Corregido')).toBeInTheDocument();
  expect(screen.getByText('Sobrantes y faltantes del periodo')).toBeInTheDocument();
  expect(screen.getByLabelText('Sobrantes y faltantes del periodo')).toHaveTextContent('Faltante−$100.00');
});
