import { afterEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { useAppUpdate } from '../../stores/appUpdate';
import { LoginPage } from './LoginPage';

function renderLogin() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><MemoryRouter><LoginPage /></MemoryRouter></Provider></QueryClientProvider>);
}

// Una versión rota que impide entrar sin tumbar la pantalla (un contrato de API cambiado) se
// arreglaba con el toast de versión nueva, que se veía también aquí. El aviso ahora vive en el riel
// (spec 064), que no existe antes de entrar: en el inicio de sesión ocupa el lugar del logo.
describe('versión nueva en el inicio de sesión', () => {
  afterEach(() => useAppUpdate.setState({ needRefresh: false, apply: () => {} }));

  test('ocupa el lugar del logo y se aplica con un toque', () => {
    const apply = vi.fn();
    useAppUpdate.setState({ needRefresh: true, apply });
    renderLogin();
    expect(screen.queryByAltText('El Gato Bobah')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Actualizar a la versión nueva' }));
    expect(apply).toHaveBeenCalledTimes(1);
  });

  test('sin versión nueva está el logo', () => {
    renderLogin();
    expect(screen.getByAltText('El Gato Bobah')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Actualizar a la versión nueva' })).toBeNull();
  });
});
