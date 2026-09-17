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
  return { ...real, listarConexiones: vi.fn(), crearConexion: vi.fn(), parejasDeLaConexion: vi.fn(), borrarConexion: vi.fn(), diferencias: vi.fn(), tiendasDisponibles: vi.fn() };
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
  vi.mocked(api.tiendasDisponibles).mockResolvedValue([
    { externalStoreId: '3806dacf-965d-4b09-91b6-6d40ad5e15db', name: 'El Gato Bobah', city: 'Monterrey', posConnected: true, alreadyAdded: false },
    { externalStoreId: '9f1c2b7e-0000-4000-8000-aaaaaaaaaaaa', name: 'El Gato Bobah Sur', city: 'Monterrey', posConnected: false, alreadyAdded: true },
  ]);
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

  // EL UUID NO SE TECLEA. Es el único dato del alta que una persona no puede producir ni deducir, y
  // pedirlo escrito es lo que deja fuera a quien nunca ha usado el sistema. Si esta prueba se cae,
  // el alta volvió a ser un campo de texto.
  it('ofrece las tiendas por nombre y ciudad, no un campo para escribir el id', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /De qué app/ }));
    await userEvent.click(await screen.findByText('Uber Eats'));
    await userEvent.click(await screen.findByRole('button', { name: /Elige tu tienda/ }));
    expect(await screen.findByText('El Gato Bobah — Monterrey')).toBeInTheDocument();
    expect(api.tiendasDisponibles).toHaveBeenCalledWith(6);
  });

  // Marcada y NO escondida: quien la busca y no la encuentra cree que se equivocó de tienda.
  it('una tienda ya conectada se marca y no deja volver a conectarla', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /De qué app/ }));
    await userEvent.click(await screen.findByText('Uber Eats'));
    await userEvent.click(await screen.findByRole('button', { name: /Elige tu tienda/ }));
    await userEvent.click(await screen.findByText('El Gato Bobah Sur — Monterrey'));
    expect(await screen.findByText(/ya está conectada/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Conectar$/ })).toBeDisabled();
  });

  // Cambiar de app dos veces seguidas puede hacer que la respuesta de la PRIMERA llegue al final.
  // Sin descartarla, la pantalla queda mostrando las tiendas de una app distinta de la elegida —con
  // sus nombres reales, así que se ve bien— y el alta conecta la tienda a la plataforma equivocada.
  it('descarta las tiendas de una app que ya no está elegida', async () => {
    let responderUber: (t: api.TiendaDePlataforma[]) => void = () => {};
    vi.mocked(api.tiendasDisponibles).mockImplementation((id) =>
      id === 6
        ? new Promise((res) => {
            responderUber = res;
          })
        : Promise.resolve([
            { externalStoreId: 'didi-1', name: 'Bobah en Didi', city: 'Monterrey', posConnected: false, alreadyAdded: false },
          ]),
    );
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /De qué app/ }));
    await userEvent.click(await screen.findByText('Uber Eats'));
    await userEvent.click(await screen.findByRole('button', { name: /Uber Eats/ }));
    await userEvent.click(await screen.findByText('Didi'));
    await screen.findByRole('button', { name: /Elige tu tienda/ });
    responderUber([
      { externalStoreId: 'uber-1', name: 'Bobah en Uber', city: 'Monterrey', posConnected: false, alreadyAdded: false },
    ]);
    await userEvent.click(screen.getByRole('button', { name: /Elige tu tienda/ }));
    expect(await screen.findByText('Bobah en Didi — Monterrey')).toBeInTheDocument();
    expect(screen.queryByText('Bobah en Uber — Monterrey')).not.toBeInTheDocument();
  });

  // La lista puede no cargar —credenciales sin configurar, la plataforma caída— y el alta no puede
  // quedar bloqueada por eso: escribir el id a mano sigue siendo la salida.
  it('si no se pueden traer las tiendas, deja escribir el id a mano', async () => {
    vi.mocked(api.tiendasDisponibles).mockRejectedValue(new Error('sin credenciales'));
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /De qué app/ }));
    await userEvent.click(await screen.findByText('Uber Eats'));
    expect(await screen.findByText(/No se pudo traer la lista/)).toBeInTheDocument();
  });

  // DOS `Page` ANIDADOS SON 48 px DE RELLENO QUE NO SEPARA NADA. La comparación de menú se pinta
  // dentro de esta pantalla, que ya está en un `Page`; cuando ella traía el suyo, la tableta perdía
  // el 8% de su alto en márgenes y el ancho se recortaba dos veces. `maxW` es la firma del `Page`.
  it('no envuelve la comparación de menú en un segundo contenedor de página', async () => {
    vi.mocked(api.listarConexiones).mockResolvedValue([
      {
        id: 1, platformId: 6, platformName: 'Uber Eats', externalStoreId: 'abc',
        label: 'Sucursal Centro', active: true, credentialsConfigured: true, lastRead: null,
      },
    ]);
    const { container } = montar();
    await screen.findByText(/Sucursal Centro/);
    const contenedores = [...container.querySelectorAll('div')].filter(
      (d) => getComputedStyle(d).maxWidth === '1150px',
    );
    expect(contenedores).toHaveLength(1);
  });

  // Conectar una tienda se hace UNA VEZ por sucursal; revisar diferencias, todos los días. El
  // formulario desplegado le cobraba ~250 px de una tableta de 600 a la tarea que sí se usa.
  it('con una tienda ya conectada el alta se guarda detrás de un botón', async () => {
    vi.mocked(api.listarConexiones).mockResolvedValue([
      {
        id: 1, platformId: 6, platformName: 'Uber Eats', externalStoreId: 'abc',
        label: 'Sucursal Centro', active: true, credentialsConfigured: true, lastRead: null,
      },
    ]);
    montar();
    await screen.findByText(/Sucursal Centro/);
    expect(screen.queryByText('Conectar una tienda')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /Conectar otra tienda/ }));
    expect(await screen.findByText('Conectar una tienda')).toBeInTheDocument();
  });
});
