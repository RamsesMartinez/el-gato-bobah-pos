import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { backofficeApi } from '../../api/backoffice';
import { ToleranciaDeDiferencia } from './ToleranciaDeDiferencia';

afterEach(() => vi.restoreAllMocks());

function pinta() {
  vi.spyOn(backofficeApi, 'cashCloseSettings').mockResolvedValue({ tolerance: '20.00' });
  render(<QueryClientProvider client={new QueryClient()}><Provider><ToleranciaDeDiferencia /></Provider></QueryClientProvider>);
}

// La tolerancia vacía no se guarda como $0 (constitución IV): borrar el campo para reescribirlo
// apagaba el control a media captura.
test('vacía no se guarda; con monto sí', async () => {
  const guardar = vi.spyOn(backofficeApi, 'setCashCloseSettings').mockResolvedValue({ tolerance: '50.00' });
  pinta();
  const campo = await screen.findByLabelText('Tolerancia de diferencia');
  await waitFor(() => expect(campo).toHaveValue('20.00'));
  await userEvent.clear(campo);
  expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
  await userEvent.type(campo, '50');
  await userEvent.click(screen.getByRole('button', { name: 'Guardar' }));
  await waitFor(() => expect(guardar).toHaveBeenCalledWith('50'));
});
