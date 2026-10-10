import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, expect, test } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { BloqueoPorInactividad } from './BloqueoPorInactividad';
import { usePosStore } from '../../stores/pos';

vi.mock('../../api/pos', () => ({
  posApi: {
    // 0 = no se bloquea; este test es sobre el estado, no sobre el temporizador.
    businessSettings: () => Promise.resolve({ lockAfterSeconds: 0 }),
    unlockOptions: () => Promise.resolve({ pinOnly: false, users: [] }),
    pinSwitch: vi.fn(),
  },
}));

// FR-002 y SC-004. Lo capturado vive en el servidor desde el primer producto (spec 030); en la
// tableta solo queda CUÁL cuenta estaba abierta. El bloqueo no puede soltarla: el operador
// volvería a una pantalla sin su cuenta, aprendería a impedir que la tableta se bloquee, y toda la
// protección se caería sin que nadie relacione una cosa con la otra.
test('bloquear no desmonta la aplicación ni suelta la cuenta abierta', async () => {
  usePosStore.getState().seleccionar({ kind: 'draft', id: 'd-1' });

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
  expect(usePosStore.getState().selected).toEqual({ kind: 'draft', id: 'd-1' });
});
