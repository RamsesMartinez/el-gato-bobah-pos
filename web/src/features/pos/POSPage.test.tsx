import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi } from 'vitest';
import { MemoryRouter } from 'react-router';
import { Provider } from '../../components/ui/provider';
import { POSPage } from './POSPage';

// El estado de caja lo decide el BACKEND (/cash-status contesta la misma regla que el cobro), así
// que aquí solo se prueba que la pantalla lo obedezca: sin turno abierto, el catálogo y el ticket
// no se muestran. Antes esto era un aviso naranja que dejaba seguir, y el operador armaba el
// pedido completo para toparse con el rechazo al cobrar, con el cliente enfrente.
const cashStatus = vi.hoisted(() => ({
  current: { open: true } as { open: boolean; deOtroDia?: boolean; openedAt?: string },
}));
vi.mock('../../api/pos', () => ({
  posApi: {
    cashStatus: () => Promise.resolve(cashStatus.current),
    menu: () => Promise.resolve({ categories: [], products: [] }),
    popular: () => Promise.resolve({ items: [] }),
    modifierDefaults: () => Promise.resolve({}),
  },
}));
vi.mock('../../hooks/useMenu', () => ({
  useMenu: () => ({ data: { categories: [], products: [] }, isLoading: false, error: null }),
}));
vi.mock('../../hooks/usePopular', () => ({ usePopular: () => ({ data: [] }) }));
vi.mock('../../hooks/useModifierDefaults', () => ({ useModifierDefaults: () => ({ data: {} }) }));

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <MemoryRouter>
          <POSPage />
        </MemoryRouter>
      </Provider>
    </QueryClientProvider>,
  );
}

test('sin caja abierta no se muestra la pantalla de venta', async () => {
  cashStatus.current = { open: false };
  montar();
  expect(await screen.findByText(/no hay caja abierta/i)).toBeInTheDocument();
  // Y lo que importa: el botón de cobrar no está por ningún lado.
  expect(screen.queryByRole('button', { name: /cobrar/i })).not.toBeInTheDocument();
});

test('con caja abierta la pantalla de venta se muestra', async () => {
  cashStatus.current = { open: true };
  montar();
  // El bloqueo desaparece; el catálogo vacío del mock no estorba para verificarlo.
  expect(await screen.findByPlaceholderText(/buscar/i)).toBeInTheDocument();
  expect(screen.queryByText(/no hay caja abierta/i)).not.toBeInTheDocument();
});

// EL AVISO DE TURNO VIEJO NO PUEDE APAGAR LA CAJA.
//
// Nace de un defecto real: un turno se quedó abierto cinco días y nada lo decía. Pero el remedio no
// puede ser peor que el mal — un negocio en operación prefiere una fecha corrida a una caja parada,
// así que el aviso informa y ofrece la acción, nunca bloquea la pantalla de venta.
test('el aviso de turno viejo se ve y NO bloquea la pantalla de venta', async () => {
  cashStatus.current = { open: true, deOtroDia: true, openedAt: '2026-08-31T18:29:00Z' };
  montar();
  expect(await screen.findByText(/la caja lleva abierta desde/i)).toBeInTheDocument();
  // Lo que de verdad importa: la pantalla de venta sigue ahí.
  expect(await screen.findByPlaceholderText(/buscar/i)).toBeInTheDocument();
  expect(screen.queryByText(/no hay caja abierta/i)).not.toBeInTheDocument();
});

// Un turno abierto HOY no molesta a nadie. Sin esto, el aviso se volvería ruido permanente y quien
// opera aprendería a ignorarlo — que es como se pierde un aviso que sí importaba.
test('un turno abierto hoy no muestra el aviso', async () => {
  cashStatus.current = { open: true, deOtroDia: false };
  montar();
  expect(await screen.findByPlaceholderText(/buscar/i)).toBeInTheDocument();
  expect(screen.queryByText(/la caja lleva abierta desde/i)).not.toBeInTheDocument();
});

// El backend puede no traer el campo todavía: el front se despliega ~7 minutos antes. Durante esos
// minutos la pantalla se queda SIN aviso, nunca rota ni con un aviso inventado.
test('sin el campo del backend no se inventa un aviso', async () => {
  cashStatus.current = { open: true };
  montar();
  expect(await screen.findByPlaceholderText(/buscar/i)).toBeInTheDocument();
  expect(screen.queryByText(/la caja lleva abierta desde/i)).not.toBeInTheDocument();
});
