import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { Provider } from '../../components/ui/provider';
import { backofficeApi } from '../../api/backoffice';
import { useSessionStore } from '../../stores/session';
import { CashPage } from './CashPage';

function Donde() {
  return <span data-testid="ruta">{useLocation().pathname}</span>;
}

function pinta(ruta: string, permissions: string[] = ['cash.operate', 'cash.configure']) {
  useSessionStore.setState({ user: { id: 1, companyId: 1, name: 'Ana', role: 'gerente', permissions } as never });
  vi.spyOn(backofficeApi, 'cashRegisters').mockResolvedValue({ items: [] });
  vi.spyOn(backofficeApi, 'allCashRegisters').mockResolvedValue({ items: [] });
  vi.spyOn(backofficeApi, 'cashHistory').mockResolvedValue({ items: [], total: 0 } as never);
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
  expect(await screen.findByTestId('ruta')).toHaveTextContent('/caja/historico');
});
