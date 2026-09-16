import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { Provider } from '../../components/ui/provider';
import { PlataformasPage } from './PlataformasPage';
import * as api from '../../api/plataformas';
import * as hookMenu from '../../hooks/useMenu';

vi.mock('../../api/plataformas', async () => {
  const real = await vi.importActual<typeof api>('../../api/plataformas');
  return { ...real, listarConexiones: vi.fn(), crearConexion: vi.fn(), parejasDeLaConexion: vi.fn(), borrarConexion: vi.fn(), diferencias: vi.fn() };
});
vi.mock('../../hooks/useMenu');

const montar = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <Provider>
          <PlataformasPage />
        </Provider>
      </MemoryRouter>
    </QueryClientProvider>,
  );

beforeEach(() => {
  vi.mocked(api.listarConexiones).mockResolvedValue([]);
  vi.mocked(api.diferencias).mockRejectedValue(new Error('sin lectura'));
  // Los ids de ESTA empresa. En el respaldo real, Uber Eats es el 6 para el negocio y el 2 para la
  // empresa de pruebas: el id NO es universal.
  vi.mocked(hookMenu.useMenu).mockReturnValue({
    data: {
      platforms: [
        { id: 5, name: 'Didi', markupPct: '35' },
        { id: 6, name: 'Uber Eats', markupPct: '35' },
        { id: 8, name: 'Propio', markupPct: '0' },
      ],
    },
  } as never);
});

describe('PlataformasPage', () => {
  // EL ID DE PLATAFORMA ES POR EMPRESA. Una lista fija en el front manda el id de otra empresa: la
  // FK compuesta lo rechaza —falla seguro— pero el formulario deja de servir sin decir por qué.
  it('ofrece las plataformas de la empresa, con sus ids reales', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /De qué app/ }));
    expect(await screen.findByText('Uber Eats')).toBeInTheDocument();
    // «Propio» es reparto del propio negocio: no tiene menú publicado que leer.
    expect(screen.queryByText('Propio')).not.toBeInTheDocument();
  });

  it('sin tiendas dadas de alta lo dice, en vez de una pantalla vacía', async () => {
    montar();
    expect(await screen.findByText(/Todavía no hay ninguna/)).toBeInTheDocument();
  });

  // Desconectar se lleva las lecturas Y el emparejamiento: hasta una sesión completa de trabajo
  // manual. La cuenta va en el diálogo, antes de confirmar.
  it('al desconectar dice cuántos platillos emparejados se pierden', async () => {
    vi.mocked(api.listarConexiones).mockResolvedValue([
      {
        id: 1, platformId: 6, platformName: 'Uber Eats', externalStoreId: 'abc',
        label: 'Sucursal Centro', active: true, credentialsConfigured: true, lastRead: null,
      },
    ]);
    vi.mocked(api.parejasDeLaConexion).mockResolvedValue(41);
    montar();
    await screen.findByText(/Sucursal Centro/);
    await userEvent.click(screen.getByRole('button', { name: /Desconectar Uber Eats/ }));
    expect(await screen.findByText(/Se pierden 41 platillos/)).toBeInTheDocument();
  });
});
