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
const cashStatus = vi.hoisted(() => ({ current: { open: true } }));
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
