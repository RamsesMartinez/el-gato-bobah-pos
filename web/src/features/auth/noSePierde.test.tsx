import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, expect, test } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { BloqueoPorInactividad } from './BloqueoPorInactividad';
import { useTicketStore } from '../../stores/ticket';

vi.mock('../../api/pos', () => ({
  posApi: {
    // 0 = no se bloquea; este test es sobre el estado, no sobre el temporizador.
    businessSettings: () => Promise.resolve({ lockAfterSeconds: 0 }),
    unlockOptions: () => Promise.resolve({ pinOnly: false, users: [] }),
    pinSwitch: vi.fn(),
  },
}));

// FR-002 y SC-004. El carrito vive en localStorage y sobrevive incluso a una recarga, así que
// sobrevive a un bloqueo por definición — HOY.
//
// El test existe para el día que alguien mueva ese estado a memoria de React "para simplificar":
// ahí el bloqueo empezaría a vaciar cuentas capturadas, el operador aprendería a impedir que la
// tableta se bloquee, y toda la protección se caería sin que nadie relacione una cosa con la otra.
test('bloquear no desmonta la aplicación ni vacía lo capturado', async () => {
  useTicketStore.getState().addLine({
    productId: 7, name: 'Alitas', unitPrice: 200, qty: 2, modifiers: [],
  });
  const antes = useTicketStore.getState().tabs[0].lines.length;
  expect(antes).toBe(1);

  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <Provider>
        <BloqueoPorInactividad><div>contenido del POS</div></BloqueoPorInactividad>
      </Provider>
    </QueryClientProvider>,
  );

  // Los hijos siguen montados: el bloqueo va ENCIMA, no en su lugar.
  expect(await screen.findByText('contenido del POS')).toBeTruthy();
  expect(useTicketStore.getState().tabs[0].lines.length).toBe(antes);
});
