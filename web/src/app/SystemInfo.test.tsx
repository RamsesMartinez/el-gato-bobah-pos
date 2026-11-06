import { beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../components/ui/provider';
import { useAppUpdate } from '../stores/appUpdate';
import { SystemInfo } from './SystemInfo';

vi.mock('../api/system', () => ({
  frontendVersion: { version: 'abc1234', builtAt: '' },
  systemApi: { backendVersion: () => Promise.resolve({ version: 'def5678', builtAt: '' }) },
}));

function renderInfo() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><SystemInfo /></Provider></QueryClientProvider>);
}

describe('el pie de sistema', () => {
  beforeEach(() => useAppUpdate.setState({ needRefresh: true, apply: () => {} }));

  // El aviso vive en el lugar del logo (spec 064). Un punto junto a la versión sería una segunda
  // señal del mismo hecho.
  test('con versión nueva no pinta un punto junto a la versión', () => {
    renderInfo();
    const pie = screen.getByTitle('Detalles del sistema');
    expect(pie.children).toHaveLength(1);
    expect(pie).toHaveTextContent('vabc1234');
  });

  test('«Detalles del sistema» sigue ofreciendo actualizar', async () => {
    renderInfo();
    fireEvent.click(screen.getByTitle('Detalles del sistema'));
    expect(await screen.findByRole('button', { name: 'Actualizar ahora' })).toBeInTheDocument();
  });
});
